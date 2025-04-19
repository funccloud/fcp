package scheme

import (
	tenancyv1alpha1 "go.funccloud.dev/fcp/api/tenancy/v1alpha1"
	workloadv1alpha1 "go.funccloud.dev/fcp/api/workload/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	knativeoperatorv1beta1 "knative.dev/operator/pkg/apis/operator/v1beta1"
	knativeservingv1 "knative.dev/serving/pkg/apis/serving/v1"
	knativeservingv1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
	// +kubebuilder:scaffold:imports
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(tenancyv1alpha1.AddToScheme(scheme))
	utilruntime.Must(workloadv1alpha1.AddToScheme(scheme))
	utilruntime.Must(knativeoperatorv1beta1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func Get() *runtime.Scheme {
	return scheme
}

// AddKnative() add knative scheme
func AddKnative() {
	utilruntime.Must(knativeservingv1beta1.AddToScheme(scheme))
	utilruntime.Must(knativeservingv1.AddToScheme(scheme))
}
