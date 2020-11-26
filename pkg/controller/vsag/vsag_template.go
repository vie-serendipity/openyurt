package vsag

import (
	"fmt"

	"github.com/google/go-cmp/cmp"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openyurtio/openyurt/pkg/controller/util"
	"github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/apis/apps/v1alpha1"
)

const (
	ContainerNameCore    = "core"
	ContainerNameSidecar = "sidecar"
	ContainerNameHelper  = "helper"

	CoreEnvSn  = "EGW_SN"
	CoreEnvKey = "EGW_KEY"
)

func newVsagDeployment(coreImage, sideCarImage, sn, key, name string, np *v1alpha1.NodePool, HaState string, sagBandwidth int) *appsv1.Deployment {
	var replicas int32 = 1
	npName := np.Name
	vsgCoreLabel := getVsagCoreLabel(npName)
	// select node in current nodepool
	nodeSelector := map[string]string{v1alpha1.LabelCurrentNodePool: npName}
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: sagNamespace,
			Labels:    vsgCoreLabel,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
			Selector: &metav1.LabelSelector{MatchLabels: vsgCoreLabel},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: vsgCoreLabel,
					// disable apparmor for vsag-core container
					Annotations: map[string]string{fmt.Sprintf("container.apparmor.security.beta.kubernetes.io/%s", ContainerNameCore): "unconfined"},
				},
				Spec: v1.PodSpec{
					Affinity: &v1.Affinity{
						PodAntiAffinity: &v1.PodAntiAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{MatchLabels: vsgCoreLabel},
									TopologyKey:   "kubernetes.io/hostname",
								},
							},
						},
					},
					Containers: []v1.Container{
						{
							Image:           coreImage,
							Name:            ContainerNameCore,
							ImagePullPolicy: v1.PullAlways,
							Env: []v1.EnvVar{
								{
									Name:  CoreEnvSn,
									Value: sn,
								},
								{
									Name:  CoreEnvKey,
									Value: key,
								},
							},
							Resources: resouresForSagBandwidth(sagBandwidth),
							SecurityContext: &v1.SecurityContext{
								Capabilities: &v1.Capabilities{Add: []v1.Capability{"NET_ADMIN", "SYS_ADMIN"}},
							},
							VolumeMounts: []v1.VolumeMount{
								{
									MountPath: "/var/sag",
									Name:      "box-route-conf",
								},
							},
						},
						{
							Image:           sideCarImage,
							Name:            ContainerNameSidecar,
							ImagePullPolicy: v1.PullAlways,
							Command:         []string{"/app/egw-sidecar"},
							Env: []v1.EnvVar{
								{
									Name:      "NODE_NAME",
									ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "spec.nodeName"}},
								},
								{
									Name:  "NODEPOOL_NAME",
									Value: npName,
								},
								{
									Name:      "POD_IP",
									ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "status.podIP"}},
								},
								{
									Name:  "HA_STATE",
									Value: HaState,
								},
								// edge-hub address
								{
									Name:  "KUBERNETES_SERVICE_HOST",
									Value: "169.254.2.1",
								},
								{
									Name:  "KUBERNETES_SERVICE_PORT",
									Value: "10261",
								},
								{
									Name:  "KUBERNETES_SERVICE_PORT_HTTPS",
									Value: "10261",
								},
								{
									Name:  "HOST_NETWORK_CONF_PATH",
									Value: "/app/host/network-scripts",
								},
								{
									Name:  "CONFIGMAP_FULL_NAME",
									Value: "kube-system/box-route-conf",
								},
							},
							Resources: v1.ResourceRequirements{
								Requests: v1.ResourceList{
									v1.ResourceCPU:    resource.MustParse("100m"),
									v1.ResourceMemory: resource.MustParse("50Mi"),
								},
								Limits: v1.ResourceList{
									v1.ResourceCPU:    resource.MustParse("200m"),
									v1.ResourceMemory: resource.MustParse("200Mi"),
								},
							},
							VolumeMounts: []v1.VolumeMount{
								{
									MountPath: "/var/egw",
									Name:      "box-route-conf",
								},
								{
									MountPath: "/app/host/network-scripts",
									Name:      "network-conf",
								},
							},
						},
					},
					NodeSelector:      nodeSelector,
					Tolerations:       getVsagCoreToleration(np),
					PriorityClassName: "system-node-critical",
					RestartPolicy:     v1.RestartPolicyAlways,
					Volumes: []v1.Volume{
						{
							Name: "network-conf",
							VolumeSource: v1.VolumeSource{
								HostPath: &v1.HostPathVolumeSource{
									Path: "/etc/sysconfig/network-scripts",
								},
							},
						},
						{
							Name: "box-route-conf",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
	}

	return deploy
}

func newVsagHelperDaemonset(helperImage, helperName, npName string) *appsv1.DaemonSet {
	//var defaultMode int32 = 420
	vsgHelperLabel := getVsagHelperLabel(npName)
	// select node in current nodepool
	nodeSelector := map[string]string{v1alpha1.LabelCurrentNodePool: npName}
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      helperName,
			Namespace: sagNamespace,
			Labels:    vsgHelperLabel,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: vsgHelperLabel},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: vsgHelperLabel,
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Image:           helperImage,
							Name:            ContainerNameHelper,
							ImagePullPolicy: v1.PullAlways,
							Command:         []string{"/app/egw-route-helper"},
							Env: []v1.EnvVar{
								{
									Name:      "NODE_NAME",
									ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "spec.nodeName"}},
								},
								{
									Name:  "NODEPOOL_NAME",
									Value: npName,
								},
								// edge-hub address
								{
									Name:  "KUBERNETES_SERVICE_HOST",
									Value: "169.254.2.1",
								},
								{
									Name:  "KUBERNETES_SERVICE_PORT",
									Value: "10261",
								},
								{
									Name:  "KUBERNETES_SERVICE_PORT_HTTPS",
									Value: "10261",
								},
								{
									Name:  "CONFIGMAP_FULL_NAME",
									Value: "kube-system/box-route-conf",
								},
								{
									Name:  "SNAT_DST_PORT",
									Value: "all",
								},
							},
							Resources: v1.ResourceRequirements{
								Requests: v1.ResourceList{
									v1.ResourceCPU:    resource.MustParse("100m"),
									v1.ResourceMemory: resource.MustParse("50Mi"),
								},
								Limits: v1.ResourceList{
									v1.ResourceCPU:    resource.MustParse("200m"),
									v1.ResourceMemory: resource.MustParse("200Mi"),
								},
							},
							SecurityContext: &v1.SecurityContext{
								Capabilities: &v1.Capabilities{Add: []v1.Capability{"NET_ADMIN"}},
							},
						},
					},
					HostNetwork:  true,
					NodeSelector: nodeSelector,
					Tolerations: []v1.Toleration{
						{
							Operator: v1.TolerationOpExists,
						},
					},
					PriorityClassName: "system-node-critical",
					RestartPolicy:     v1.RestartPolicyAlways,
				},
			},
		},
	}

	return ds
}

func getVsagCoreLabel(npName string) map[string]string {
	return map[string]string{util.K8sAppLabelKey: util.VsagCoreAppLabelVal, util.NodepoolLabelKey: npName, util.NodeLabelArchKey: util.NodeArchValueAmd64}
}

func getVsagHelperLabel(npName string) map[string]string {
	return map[string]string{util.K8sAppLabelKey: util.VsagHelperAppLabelVal, util.NodepoolLabelKey: npName, util.NodeLabelArchKey: util.NodeArchValueAmd64}
}

func getVsagCoreToleration(np *v1alpha1.NodePool) []v1.Toleration {
	// toleration 10s for node not ready
	var defaultTolerationSeconds int64 = 10
	ret := []v1.Toleration{
		{
			Effect:            v1.TaintEffectNoExecute,
			Operator:          v1.TolerationOpExists,
			Key:               v1.TaintNodeNotReady,
			TolerationSeconds: &defaultTolerationSeconds,
		},
		{
			Effect:            v1.TaintEffectNoExecute,
			Operator:          v1.TolerationOpExists,
			Key:               v1.TaintNodeUnreachable,
			TolerationSeconds: &defaultTolerationSeconds,
		},
	}
	if np != nil && len(np.Spec.Taints) > 0 {
		for _, taint := range np.Spec.Taints {
			ret = append(ret, v1.Toleration{
				Key:      taint.Key,
				Operator: v1.TolerationOpEqual,
				Value:    taint.Value,
				Effect:   taint.Effect,
			})
		}
	}
	return ret
}
func resourceRequirementsEquals(d1, d2 *appsv1.Deployment, container string) bool {
	var c1, c2 v1.Container
	for _, c := range d1.Spec.Template.Spec.Containers {
		if c.Name == container {
			c1 = c
		}
	}
	for _, c := range d2.Spec.Template.Spec.Containers {
		if c.Name == container {
			c2 = c
		}
	}
	return cmp.Equal(c1.Resources, c2.Resources)
}

func isTolerationsEqual(old, new []v1.Toleration) bool {
	for _, o := range old {
		exist := false
		for _, n := range new {
			if o.Key == n.Key && o.Value == n.Value && o.Operator == n.Operator && o.Effect == n.Effect {
				exist = true
				break
			}
		}
		if !exist {
			return false
		}
	}
	for _, n := range new {
		exist := false
		for _, o := range old {
			if n.Key == o.Key && n.Value == o.Value && n.Operator == o.Operator && n.Effect == o.Effect {
				exist = true
				break
			}
		}
		if !exist {
			return false
		}
	}
	return true
}

func resouresForSagBandwidth(sagBandwidth int) v1.ResourceRequirements {
	// resource request for vsag-core
	//  bandwidth(M): 0~100, cpu=250m, mem=500Mi
	//  bandwidth(M): 100~200, cpu=500m, mem=1Gi
	//  bandwidth(M):  200~500, cpu=1000m, mem=2Gi
	//  bandwidth(M):  500~1000, cpu=2000m, mem=4Gi
	// ref: https://help.aliyun.com/document_detail/182147.html
	var cpuReq, memReq, cpuLimit, memLimit string
	if sagBandwidth <= 100 {
		cpuReq = "250m"
		memReq = "500Mi"
		cpuLimit = "1000m"
		memLimit = "2Gi"
	}
	if sagBandwidth > 100 && sagBandwidth <= 200 {
		cpuReq = "500m"
		memReq = "1Gi"
		cpuLimit = "1000m"
		memLimit = "2Gi"
	}
	if sagBandwidth > 200 && sagBandwidth <= 500 {
		cpuReq = "1000m"
		memReq = "2Gi"
		cpuLimit = "2000m"
		memLimit = "4Gi"
	}
	if sagBandwidth > 500 {
		cpuReq = "2000m"
		memReq = "4Gi"
		cpuLimit = "4000m"
		memLimit = "8Gi"
	}

	return v1.ResourceRequirements{
		Requests: v1.ResourceList{
			v1.ResourceCPU:    resource.MustParse(cpuReq),
			v1.ResourceMemory: resource.MustParse(memReq),
		},
		Limits: v1.ResourceList{
			v1.ResourceCPU:    resource.MustParse(cpuLimit),
			v1.ResourceMemory: resource.MustParse(memLimit),
		},
	}
}
