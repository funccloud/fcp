package knative

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"go.funccloud.dev/fcp/internal/scheme"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta" // Added import
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CheckOrInstallVersion checks if Knative Serving (managed by Operator) is installed and ready.
// If not installed or not ready, it attempts to install using the Knative Operator.
// Returns an error if the check fails or if installation is required and fails.
func CheckOrInstallVersion(ctx context.Context, domain string, k8sClient client.Client, log logr.Logger) error {
	log = log.WithName("KnativeCheckInstall")

	// Check for the KnativeServing CR status first, as this indicates Operator success
	knativeServingNN := types.NamespacedName{Namespace: knativeServingNamespace, Name: knativeServingCRName}
	ks := &unstructured.Unstructured{}
	ks.SetGroupVersionKind(schema.GroupVersionKind{ // Use schema from installer.go
		Group:   "operator.knative.dev",
		Version: "v1beta1",
		Kind:    "KnativeServing",
	})

	log.Info("Checking KnativeServing CR status...", "namespace", knativeServingNN.Namespace, "name", knativeServingNN.Name)
	err := k8sClient.Get(ctx, knativeServingNN, ks)

	needsInstall := false
	if err == nil {
		// KnativeServing CR exists, check its Ready status
		conditions, found, _ := unstructured.NestedSlice(ks.Object, "status", "conditions")
		if found {
			isReady := false
			for _, c := range conditions {
				condition, ok := c.(map[string]interface{})
				if !ok {
					continue
				}
				condType, typeFound := condition["type"].(string)
				condStatus, statusFound := condition["status"].(string)

				if typeFound && statusFound && condType == "Ready" {
					if condStatus == string(metav1.ConditionTrue) {
						log.Info("Knative Serving (managed by Operator) is installed and Ready.")
						scheme.AddKnative() // Add Knative scheme to the runtime scheme
						return nil          // Already installed and ready
					}
					// Found Ready condition, but it's not True
					log.Info("KnativeServing CR found but not Ready.", "status", condStatus)
					isReady = false // Explicitly mark as not ready if condition found but not True
					break           // No need to check other conditions if Ready is found
				}
			}
			// If Ready condition was found but wasn't True
			if !isReady {
				log.Info("KnativeServing CR Ready condition is not True. Will attempt installation/reconciliation.")
				needsInstall = true
			}
		} else {
			log.Info("KnativeServing CR found but status.conditions not found. Will attempt installation/reconciliation.")
			needsInstall = true
		}
	} else if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) { // Modified condition: Check for IsNotFound OR IsNoMatchError
		// CR or CRD not found.
		log.Info("Knative Serving CRD or CR not found. Attempting installation...")
		needsInstall = true
	} else {
		// Another error occurred while fetching the KnativeServing CR
		log.Error(err, "Error checking KnativeServing CR")
		return fmt.Errorf("error checking KnativeServing CR: %w", err)
	}

	if needsInstall {
		log.Info("Attempting Knative Serving installation/reconciliation...")
		installErr := InstallKnative(ctx, domain, k8sClient, log)
		if installErr != nil {
			log.Error(installErr, "Failed to install/reconcile Knative Serving using Operator")
			return fmt.Errorf("failed to install/reconcile Knative Serving using Operator: %w", installErr)
		}
		log.Info("Knative Serving (managed by Operator) installation/reconciliation process completed successfully.")
	}

	scheme.AddKnative() // Add Knative scheme to the runtime scheme (safe to call multiple times)
	return nil          // Successful installation or already existed and ready
}
