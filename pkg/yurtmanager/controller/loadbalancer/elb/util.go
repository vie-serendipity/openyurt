package elb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"
)

type RequestContext struct {
	Ctx       context.Context
	ClusterId string
	Service   *v1.Service
	AnnoCtx   *AnnotationContext
	Recorder  record.EventRecorder
}

func GetDefaultLoadBalancerName(reqCtx *RequestContext) string {
	if reqCtx.Service == nil {
		return ""
	}
	ret := "a" + string(reqCtx.Service.UID)
	ret = strings.Replace(ret, "-", "", -1)
	//AWS requires that the name of a load balancer is shorter than 32 bytes.
	if len(ret) > 32 {
		ret = ret[:32]
	}
	return ret
}

func GetCID(reqCtx *RequestContext) string {
	return reqCtx.ClusterId
}

func NamespacedName(obj metav1.Object) types.NamespacedName {
	return types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
}

func Key(obj metav1.Object) string {
	return fmt.Sprintf("%s/%s", obj.GetNamespace(), obj.GetName())
}

var re = regexp.MustCompile(".*(Message:.*)")

func GetLogMessage(err error) string {
	if err == nil {
		return ""
	}

	attr := strings.Split(err.Error(), "[SDKError]")
	if len(attr) < 2 {
		return err.Error()
	}

	sub := re.FindStringSubmatch(attr[1])
	if len(sub) > 1 {
		return sub[1]
	}

	return err.Error()
}

func PrettyJson(object interface{}) string {
	b, err := json.MarshalIndent(object, "", "    ")
	if err != nil {
		fmt.Printf("ERROR: PrettyJson, %v\n %s\n", err, b)
	}
	return string(b)
}

// MergeStringMap will merge multiple map[string]string into single one.
// The merge is executed for maps argument in sequential order, if a key already exists, the value from previous map is kept.
// e.g. MergeStringMap(map[string]string{"a": "1", "b": "2"}, map[string]string{"a": "3", "d": "4"}) == map[string]string{"a": "1", "b": "2", "d": "4"}
func MergeStringMap(maps ...map[string]string) map[string]string {
	ret := make(map[string]string)
	for _, _map := range maps {
		for k, v := range _map {
			if _, ok := ret[k]; !ok {
				ret[k] = v
			}
		}
	}
	return ret
}
func RetryImmediateOnError(interval time.Duration, timeout time.Duration, retryable func(error) bool, fn func() error) error {
	return wait.PollImmediate(interval, timeout, func() (bool, error) {
		err := fn()
		if err != nil {
			if retryable(err) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	})
}

func IsStringSliceEqual(s1, s2 []string) bool {
	if len(s1) != len(s2) {
		return false
	}

	for _, i := range s1 {
		found := false
		for _, j := range s2 {
			if strings.EqualFold(i, j) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func LogEndpointSlice(es *discovery.EndpointSlice) string {
	if es == nil {
		return "endpointSlice is nil"
	}
	var epAddrList []string
	for _, ep := range es.Endpoints {
		epAddrList = append(epAddrList, ep.Addresses...)
	}

	return strings.Join(epAddrList, ",")
}

func LogEndpointSliceList(esList []discovery.EndpointSlice) string {
	if esList == nil {
		return "endpointSliceList is nil"
	}
	var epAddrList []string
	for _, es := range esList {
		for _, ep := range es.Endpoints {
			if ep.Conditions.Ready != nil && !*ep.Conditions.Ready {
				continue
			}
			epAddrList = append(epAddrList, ep.Addresses...)
		}
	}

	return strings.Join(epAddrList, ",")
}

const TRY_AGAIN = "try again"

func Retry(
	backoff *wait.Backoff,
	fun func(svc *v1.Service) error,
	svc *v1.Service,
) error {
	if backoff == nil {
		backoff = &wait.Backoff{
			Duration: 1 * time.Second,
			Steps:    8,
			Factor:   2,
			Jitter:   4,
		}
	}
	return wait.ExponentialBackoff(
		*backoff,
		func() (bool, error) {
			err := fun(svc)
			if err != nil &&
				strings.Contains(err.Error(), TRY_AGAIN) {
				klog.Errorf("retry with error: %s", err.Error())
				return false, nil
			}
			if err != nil {
				klog.Errorf("retry error: NotRetry, %s", err.Error())
			}
			return true, nil
		},
	)
}

func GetServiceHash(svc *v1.Service) string {
	var op []interface{}
	// ServiceSpec
	op = append(op, svc.Spec.Ports, svc.Spec.Type, svc.Spec.ExternalTrafficPolicy, svc.Spec.LoadBalancerClass)
	op = append(op, svc.Annotations, svc.DeletionTimestamp)
	return HashObject(op)
}

const ReconcileHashLable = "alibabacloud.com/reconcile.hash"

// HashObject
// Entrance for object computeHash
func HashObject(o interface{}) string {
	data, _ := json.Marshal(o)
	var a interface{}
	err := json.Unmarshal(data, &a)
	if err != nil {
		klog.Errorf("unmarshal: %s", err.Error())
	}
	remove(&a)
	return computeHash(PrettyYaml(a))
}
func HashString(o interface{}) string {
	data, _ := json.Marshal(o)
	var a interface{}
	err := json.Unmarshal(data, &a)
	if err != nil {
		klog.Errorf("unmarshal: %s", err.Error())
	}
	remove(&a)
	return PrettyYaml(a)
}

func remove(v *interface{}) {
	o := *v
	switch o := o.(type) {
	case []interface{}:
		under := o
		// remove empty object

		for _, m := range under {
			remove(&m)
		}
		var emit []interface{}
		for _, m := range under {
			// remove empty under object
			if isUnderlyingTypeZero(m) {
				continue
			}
			emit = append(emit, m)
		}
		*v = emit
	case map[string]interface{}:
		me := o
		for k, v := range me {
			if isHashLabel(k) {
				delete(me, k)
				continue
			}
			if isUnderlyingTypeZero(v) {
				delete(me, k)
			} else {
				// continue on next value
				remove(&v)
			}
		}
		*v = o
	default:
	}
}

func isUnderlyingTypeZero(x interface{}) bool {
	if x == nil {
		return true
	}
	v := reflect.ValueOf(x)
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice:
		return v.Len() == 0
	}

	zero := reflect.Zero(reflect.TypeOf(x)).Interface()
	return reflect.DeepEqual(x, zero)
}

func isHashLabel(k string) bool {
	return k == ReconcileHashLable
}

func PrettyYaml(obj interface{}) string {
	bs, err := yaml.Marshal(obj)
	if err != nil {
		klog.Errorf("failed to parse yaml, %v", err.Error())
	}
	return string(bs)
}

func computeHash(target string) string {
	hash := sha256.Sum224([]byte(target))
	return strings.ToLower(hex.EncodeToString(hash[:]))
}

type NodePoolAffinity struct {
	NodePoolSelectorTerms []NodePoolSelectorTerm `json:"nodePoolSelectorTerms"`
}

type NodePoolSelectorTerm struct {
	LabelMatchExpressions []MatchExpression `json:"labelMatchExpressions,omitempty"`
	FiledMatchExpressions []MatchExpression `json:"filedMatchExpressions,omitempty"`
}

type MatchExpression struct {
	Key      string   `json:"key,omitempty"`
	Operator string   `json:"operator,omitempty"`
	Values   []string `json:"values,omitempty"`
}

func Unmarshal(str string) (*NodePoolAffinity, error) {
	var nodePoolAffinity NodePoolAffinity
	err := json.Unmarshal([]byte(str), &nodePoolAffinity)
	if err != nil {
		klog.Errorf("unmarshal string %s error %s", str, err.Error())
		return nil, err
	}
	return &nodePoolAffinity, nil
}
