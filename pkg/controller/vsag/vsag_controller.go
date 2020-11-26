package vsag

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/go-cmp/cmp"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	informers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/component-base/metrics/prometheus/ratelimiter"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/controller/util"
	nodeutil "github.com/openyurtio/openyurt/pkg/controller/util/node"
	"github.com/openyurtio/openyurt/pkg/controller/vsag/cloud"
	appsv1alpha1 "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/apis/apps/v1alpha1"
	appsclientset "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/clientset/versioned"
	yurtscheme "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/clientset/versioned/scheme"
	appsinformers "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/informers/externalversions/apps/v1alpha1"
	appslisters "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/listers/apps/v1alpha1"
)

const (
	sagUnbindGracePeriod = 5 * time.Second
	sagNamespace         = metav1.NamespaceSystem
	sagDefaultRegion     = "cn-shanghai"

	LabelNodeRoleExcludeNode = "service.alibabacloud.com/exclude-node"
)

var once sync.Once

// Controller is the controller that manages vsag state.
type Controller struct {
	nodeLister             listers.NodeLister
	nodeInformerSynced     cache.InformerSynced
	nodepoolLister         appslisters.NodePoolLister
	nodepoolInformerSynced cache.InformerSynced
	kubeClient             kubernetes.Interface
	appsClient             appsclientset.Interface
	recorder               record.EventRecorder
	vsagCoreImage          string
	vsagSidecarImage       string
	vsagHelperImage        string
	cloudClientMgr         *cloud.ClientMgr
	uID                    string
	clusterID              string
	clusterRegion          string
	vpcID                  string
	cenID                  string

	npWorkQueue   workqueue.RateLimitingInterface
	nodeWorkQueue workqueue.RateLimitingInterface
	workers       int

	// check if any edge nodepool is using vsag
	isVsagEnabled bool

	// record nodes with cen route synced status
	cloudNodeCache map[string]bool
	nodeCacheLock  sync.Mutex

	// nodepool cache, key: np name, value: nodepool data
	nodepoolCache     map[string]*appsv1alpha1.NodePool
	nodepoolCacheLock sync.Mutex
}

func NewVsagController(
	nodeInformer informers.NodeInformer,
	nodepoolInformer appsinformers.NodePoolInformer,
	kubeClient kubernetes.Interface,
	appsClient appsclientset.Interface,
	cloudConfig *util.CloudConfig,
	guestCloudConfig *util.CCMCloudConfig,
	vsagCoreImage string,
	vsagSidecarImage string,
	vsagHelperImage string,
	workers int,
) (*Controller, error) {

	if kubeClient == nil || appsClient == nil {
		klog.Fatalf("kubeClient is nil when starting vsag Controller")
	}

	eventBroadcaster := record.NewBroadcaster()
	utilruntime.Must(yurtscheme.AddToScheme(scheme.Scheme))
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: "egw-controller"})
	eventBroadcaster.StartLogging(klog.Infof)

	klog.Infof("Sending events to api server.")
	eventBroadcaster.StartRecordingToSink(
		&v1core.EventSinkImpl{
			Interface: kubeClient.CoreV1().Events(""),
		})

	if kubeClient.CoreV1().RESTClient().GetRateLimiter() != nil {
		ratelimiter.RegisterMetricAndTrackRateLimiterUsage("vsag_controller", kubeClient.CoreV1().RESTClient().GetRateLimiter())
	}

	vc := &Controller{
		nodeLister:             nodeInformer.Lister(),
		nodeInformerSynced:     nodeInformer.Informer().HasSynced,
		nodepoolLister:         nodepoolInformer.Lister(),
		nodepoolInformerSynced: nodepoolInformer.Informer().HasSynced,
		kubeClient:             kubeClient,
		appsClient:             appsClient,
		recorder:               recorder,
		vsagCoreImage:          vsagCoreImage,
		vsagSidecarImage:       vsagSidecarImage,
		vsagHelperImage:        vsagHelperImage,
		cloudClientMgr:         cloud.NewClientMgrSet(cloudConfig, guestCloudConfig),
		uID:                    guestCloudConfig.Global.UID,
		clusterID:              guestCloudConfig.Global.ClusterID,
		vpcID:                  guestCloudConfig.Global.VpcID,
		clusterRegion:          guestCloudConfig.Global.Region,
		npWorkQueue:            workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "vsag_controller_nodepools"),
		nodeWorkQueue:          workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "vsag_controller_nodes"),
		workers:                workers,
		isVsagEnabled:          false,
		cloudNodeCache:         make(map[string]bool),
		nodepoolCache:          make(map[string]*appsv1alpha1.NodePool),
	}

	// watch node for cen route syncing
	nodeInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			node, ok := obj.(*v1.Node)
			if !ok {
				utilruntime.HandleError(fmt.Errorf("unexpected object type: %v", obj))
				return
			}
			vc.enqueueNode(node)
		},
		UpdateFunc: func(_, newObj interface{}) {
			node, ok := newObj.(*v1.Node)
			if !ok {
				utilruntime.HandleError(fmt.Errorf("unexpected object type: %v", newObj))
				return
			}
			vc.enqueueNode(node)
		},
		DeleteFunc: func(obj interface{}) {
			node, ok := obj.(*v1.Node)
			if !ok {
				tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
				if !ok {
					utilruntime.HandleError(fmt.Errorf("unexpected object type: %v", obj))
					return
				}
				if node, ok = tombstone.Obj.(*v1.Node); !ok {
					utilruntime.HandleError(fmt.Errorf("unexpected object type: %v", obj))
					return
				}
			}
			vc.enqueueNode(node)
		},
	})

	// watch nodepool for vsag configuration
	nodepoolInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			np, ok := obj.(*appsv1alpha1.NodePool)
			if !ok {
				utilruntime.HandleError(fmt.Errorf("unexpected object type: %v", obj))
				return
			}
			vc.enqueueNodepool(np)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldNp, ok := oldObj.(*appsv1alpha1.NodePool)
			if !ok {
				utilruntime.HandleError(fmt.Errorf("unexpected object type: %v", oldObj))
				return
			}
			newNp, ok := newObj.(*appsv1alpha1.NodePool)
			if !ok {
				utilruntime.HandleError(fmt.Errorf("unexpected object type: %v", newObj))
				return
			}
			if oldNp.DeletionTimestamp != nil || newNp.DeletionTimestamp != nil {
				vc.enqueueNodepool(newNp)
			}
			if !cmp.Equal(oldNp.Annotations, newNp.Annotations, cmp.Transformer("omit", func(annotations map[string]string) map[string]string {
				return map[string]string{util.NodePoolSagBandwidthAnnotation: annotations[util.NodePoolSagBandwidthAnnotation]}
			})) {
				vc.enqueueNodepool(newNp)
			}
		},
		DeleteFunc: func(obj interface{}) {
			np, ok := obj.(*appsv1alpha1.NodePool)
			if !ok {
				tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
				if !ok {
					utilruntime.HandleError(fmt.Errorf("unexpected object type: %v", obj))
					return
				}
				if np, ok = tombstone.Obj.(*appsv1alpha1.NodePool); !ok {
					utilruntime.HandleError(fmt.Errorf("unexpected object type: %v", obj))
					return
				}
			}
			vc.enqueueNodepool(np)
		},
	})

	return vc, nil
}

// Run starts an asynchronous loop that syncing the vsag.
func (vc *Controller) Run(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()

	klog.Infof("Starting vsag controller")
	defer klog.Infof("Shutting down vsag controller")

	if !cache.WaitForNamedCacheSync("vsag", stopCh, vc.nodepoolInformerSynced, vc.nodeInformerSynced) {
		return
	}

	// Close work queue to cleanup go routine.
	defer vc.npWorkQueue.ShutDown()
	defer vc.nodeWorkQueue.ShutDown()

	go wait.Until(vc.nodeWorker, time.Second, stopCh)

	for i := 0; i < vc.workers; i++ {
		go wait.Until(vc.nodepoolWorker, time.Second, stopCh)
	}

	<-stopCh
}

func (vc *Controller) enqueueNode(obj interface{}) {
	node, _ := obj.(*v1.Node)
	// skip deleted node, as the route will be deleted by ccm
	if node.DeletionTimestamp != nil {
		return
	}
	// skip edge node
	if !nodeutil.IsCloudNode(node) {
		return
	}
	// exclude ccm not managed node
	if _, exclude := node.Labels[LabelNodeRoleExcludeNode]; exclude {
		klog.V(4).Infof("Node %s is annotated to be service excluded, skip it", node.Name)
		return
	}
	// skip node with no cidrs allocated
	if vc.getNodeCIDR(node) == "" {
		klog.V(4).Infof("Node %s has no pod cidr allocated, skip it", node.Name)
		return
	}
	// skip node with node cloud provider ID
	if node.Spec.ProviderID == "" {
		klog.V(4).Infof("Node %s has no Provider ID, skip it", node.Name)
		return
	}

	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %#v: %v", obj, err))
		return
	}

	klog.V(4).Infof("enqueue node %s", key)
	vc.nodeWorkQueue.AddRateLimited(key)
}

func (vc *Controller) enqueueNodepool(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %#v: %v", obj, err))
		return
	}

	klog.V(4).Infof("enqueue nodepool %s", key)
	vc.npWorkQueue.AddRateLimited(key)
}
