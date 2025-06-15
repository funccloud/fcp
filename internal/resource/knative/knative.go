package knative

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"go.funccloud.dev/fcp/internal/scheme"
	"go.funccloud.dev/fcp/internal/yamlutil"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:embed le-prod-issuer.yaml
var leProdIssuerYAML string

//go:embed le-staging-issuer.yaml
var leStagingIssuerYAML string

// CheckOrInstallVersion checks if Knative Serving (managed by Operator) is installed and ready.
// If not installed or not ready, it attempts to install using the Knative Operator after applying the appropriate Let's Encrypt issuer.
// Returns an error if the check fails or if installation is required and fails.
func CheckOrInstallVersion(ctx context.Context, domain string, k8sClient client.Client, ioStreams genericiooptions.IOStreams, isKind bool) (string, error) { // Added isKind parameter

	// Check for the KnativeServing CR status first, as this indicates Operator success
	// Ensure knativeServingNamespace and knativeServingCRName are accessible from installer.go (same package)
	knativeServingNN := types.NamespacedName{Namespace: knativeServingNamespace, Name: knativeServingCRName}
	ks := &unstructured.Unstructured{}
	ks.SetGroupVersionKind(schema.GroupVersionKind{ // Use schema from installer.go
		Group:   "operator.knative.dev",
		Version: "v1beta1",
		Kind:    "KnativeServing",
	})

	_, _ = fmt.Fprintln(ioStreams.Out, "Checking KnativeServing CR status...", "namespace", knativeServingNN.Namespace, "name", knativeServingNN.Name)
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
						_, _ = fmt.Fprintln(ioStreams.Out, "Knative Serving (managed by Operator) is installed and Ready.")
						scheme.AddKnative() // Add Knative scheme to the runtime scheme
						return "", nil      // Already installed and ready
					}
					// Found Ready condition, but it's not True
					_, _ = fmt.Fprintln(ioStreams.Out, "KnativeServing CR found but not Ready.", "status", condStatus)
					isReady = false // Explicitly mark as not ready if condition found but not True
					break           // No need to check other conditions if Ready is found
				}
			}
			// If Ready condition was found but wasn't True
			if !isReady {
				_, _ = fmt.Fprintln(ioStreams.Out, "KnativeServing CR Ready condition is not True. Will attempt installation/reconciliation.")
				needsInstall = true
			}
		} else {
			_, _ = fmt.Fprintln(ioStreams.Out, "KnativeServing CR found but status.conditions not found. Will attempt installation/reconciliation.")
			needsInstall = true
		}
	} else if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) { // Modified condition: Check for IsNotFound OR IsNoMatchError
		// CR or CRD not found.
		_, _ = fmt.Fprintln(ioStreams.Out, "Knative Serving CRD or CR not found. Attempting installation...")
		needsInstall = true
	} else {
		// Another error occurred while fetching the KnativeServing CR
		_, _ = fmt.Fprintln(ioStreams.ErrOut, "Error checking KnativeServing CR", "error", err)
		return "", fmt.Errorf("error checking KnativeServing CR: %w", err)
	}
	var issuerName string
	if needsInstall {
		// Apply the appropriate Let's Encrypt issuer before installing Knative
		var issuerYAML string
		if isKind || domain == "localhost" || strings.HasSuffix(domain, ".local") {
			_, _ = fmt.Fprintln(ioStreams.Out, "Applying Let's Encrypt staging issuer for Kind cluster...")
			issuerYAML = leStagingIssuerYAML
			issuerName = "le-staging-issuer" // Assuming name from YAML
		} else {
			_, _ = fmt.Fprintln(ioStreams.Out, "Applying Let's Encrypt production issuer...")
			issuerYAML = leProdIssuerYAML
			issuerName = "le-prod-issuer" // Assuming name from YAML
		}

		applyErr := yamlutil.ApplyManifestYAML(ctx, k8sClient, issuerYAML, ioStreams)
		if applyErr != nil {
			_, _ = fmt.Fprintln(ioStreams.ErrOut, "Failed to apply Let's Encrypt issuer", "issuer", issuerName, "error", applyErr)
			return "", fmt.Errorf("failed to apply Let's Encrypt issuer %s: %w", issuerName, applyErr)
		}
		_, _ = fmt.Fprintln(ioStreams.Out, "Successfully applied Let's Encrypt issuer", "issuer", issuerName)

		_, _ = fmt.Fprintln(ioStreams.Out, "Attempting Knative Serving installation/reconciliation...")
		installErr := InstallKnative(ctx, domain, issuerName, isKind, k8sClient, ioStreams)
		if installErr != nil {
			_, _ = fmt.Fprintln(ioStreams.ErrOut, "Failed to install/reconcile Knative Serving using Operator", "error", installErr)
			return "", fmt.Errorf("failed to install/reconcile Knative Serving using Operator: %w", installErr)
		}
		_, _ = fmt.Fprintln(ioStreams.Out, "Knative Serving (managed by Operator) installation/reconciliation process completed successfully.")
	}

	scheme.AddKnative()    // Add Knative scheme to the runtime scheme (safe to call multiple times)
	return issuerName, nil // Successful installation or already existed and ready
}
