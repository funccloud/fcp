package certmanager

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.funccloud.dev/fcp/internal/yamlutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

const (
	CertManagerVersion             = "v1.17.1"
	CertManagerManifestURLTemplate = "https://github.com/cert-manager/cert-manager/releases/download/%s/cert-manager.yaml"
	CertManagerCRDsURLTemplate     = "https://github.com/cert-manager/cert-manager/releases/download/%s/" +
		"cert-manager.crds.yaml"
)

// InstallCertManager attempts to install cert-manager by downloading its CRDs and main manifests and applying them.
func InstallCertManager(ctx context.Context, k8sClient client.Client, ioStreams genericiooptions.IOStreams) error {
	_, _ = fmt.Fprintln(ioStreams.Out, "Cert-manager not found, attempting installation...", "version", CertManagerVersion)

	// 1. Install CRDs
	crdsURL := fmt.Sprintf(CertManagerCRDsURLTemplate, CertManagerVersion)
	_, _ = fmt.Fprintln(ioStreams.Out, "Downloading cert-manager CRDs manifest", "url", crdsURL)
	if err := yamlutil.ApplyManifestFromURL(ctx, k8sClient, ioStreams, crdsURL); err != nil {
		_, _ = fmt.Fprintln(ioStreams.ErrOut, "Failed to apply cert-manager CRDs manifest", "error", err)
		return fmt.Errorf("failed to apply cert-manager CRDs from %s: %w", crdsURL, err)
	}
	_, _ = fmt.Fprintln(ioStreams.Out, "Cert-manager CRDs manifest applied successfully.")

	// Brief pause to allow CRDs to be established in the API server
	_, _ = fmt.Fprintln(ioStreams.Out, "Waiting briefly for CRDs to be established...")
	time.Sleep(10 * time.Second)

	// 2. Install main cert-manager components
	manifestURL := fmt.Sprintf(CertManagerManifestURLTemplate, CertManagerVersion)
	_, _ = fmt.Fprintln(ioStreams.Out, "Downloading main cert-manager manifest", "url", manifestURL)
	manifestBytes, err := yamlutil.DownloadYAMLFromURL(ctx, manifestURL, ioStreams)
	if err != nil {
		_, _ = fmt.Fprintln(ioStreams.ErrOut, "Failed to download main cert-manager manifest", "error", err)
		return fmt.Errorf("failed to download main cert-manager manifest from %s: %w", manifestURL, err)
	}
	manifestString := string(manifestBytes)
	// leader election namespace is hardcoded to "kube-system" in the manifest, replace it with CertManagerNamespace
	// This is necessary to ensure fcp will be able to run in autopilot clusters like gke autopilot
	// that we not have access to kube-system namespace.
	manifestString = strings.ReplaceAll(manifestString, "kube-system", CertManagerNamespace)

	// Add tolerations to Deployments in the cert-manager manifest
	_, _ = fmt.Fprintln(ioStreams.Out, "Adding tolerations to cert-manager manifest...")
	modifiedManifestWithTolerations, err := addTolerationsToManifest(manifestString, ioStreams)
	if err != nil {
		_, _ = fmt.Fprintln(ioStreams.ErrOut, "Failed to add tolerations to cert-manager manifest", "error", err)
		return fmt.Errorf("failed to add tolerations to cert-manager manifest: %w", err)
	}
	manifestString = modifiedManifestWithTolerations

	if err := yamlutil.ApplyManifestYAML(ctx, k8sClient, manifestString, ioStreams); err != nil {
		_, _ = fmt.Fprintln(ioStreams.ErrOut, "Failed to apply main cert-manager manifest", "error", err)
		return fmt.Errorf("failed to apply main cert-manager manifest from %s: %w", manifestURL, err)
	}
	_, _ = fmt.Fprintln(ioStreams.Out, "Main cert-manager manifest applied successfully.")

	// 3. Wait for deployments to become ready
	_, _ = fmt.Fprintln(ioStreams.Out, "Waiting for cert-manager deployments to become ready...")
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Minute) // 5-minute timeout
	defer cancel()
	err = waitForCertManagerDeployments(waitCtx, k8sClient, ioStreams)
	if err != nil {
		_, _ = fmt.Fprintln(ioStreams.ErrOut, "Cert-manager deployments did not become ready in time", "error", err)
		return fmt.Errorf("cert-manager deployments did not become ready: %w", err)
	}

	_, _ = fmt.Fprintln(ioStreams.Out, "Cert-manager installation completed successfully.")
	return nil
}

// waitForCertManagerDeployments waits for the main cert-manager deployments to be available.
func waitForCertManagerDeployments(ctx context.Context, k8sClient client.Client, ioStreams genericiooptions.IOStreams) error {
	deployments := []string{CertManagerDeployment, "cert-manager-webhook", "cert-manager-cainjector"}

	for _, depName := range deployments {
		_, _ = fmt.Fprintln(ioStreams.Out, "Waiting for deployment", "deployment", depName, "namespace", CertManagerNamespace)
		err := wait.PollUntilContextCancel(ctx, 5*time.Second, true, func(ctx context.Context) (bool, error) {
			ready, err := isDeploymentReady(ctx, k8sClient, CertManagerNamespace, depName)
			if err != nil {
				// If not found yet, keep waiting
				if apierrors.IsNotFound(err) {
					_, _ = fmt.Fprintln(ioStreams.Out, "Deployment not found yet, waiting...", "deployment", depName) // V(1) equivalent
					return false, nil
				}
				_, _ = fmt.Fprintln(ioStreams.ErrOut, "Error checking deployment status", "deployment", depName, "error", err)
				return false, err // Real error, stop waiting
			}
			if ready {
				_, _ = fmt.Fprintln(ioStreams.Out, "Deployment is ready", "deployment", depName)
			}
			return ready, nil
		})

		if err != nil {
			return fmt.Errorf("error waiting for deployment %s/%s: %w", CertManagerNamespace, depName, err)
		}
	}
	return nil
}

// isDeploymentReady checks if a specific deployment is ready (available).
func isDeploymentReady(ctx context.Context, k8sClient client.Client, namespace, name string) (bool, error) {
	deployment := &appsv1.Deployment{}
	namespacedName := types.NamespacedName{Namespace: namespace, Name: name}

	err := k8sClient.Get(ctx, namespacedName, deployment)
	if err != nil {
		return false, err // Return the error (including NotFound)
	}

	// Check if the number of ready replicas equals the desired number
	// and if the observed generation is the latest.
	if deployment.Spec.Replicas != nil &&
		deployment.Status.ObservedGeneration >= deployment.Generation &&
		deployment.Status.UpdatedReplicas == *deployment.Spec.Replicas &&
		deployment.Status.Replicas == *deployment.Spec.Replicas &&
		deployment.Status.AvailableReplicas == *deployment.Spec.Replicas {
		// Check deployment conditions
		for _, cond := range deployment.Status.Conditions {
			if cond.Type == appsv1.DeploymentAvailable && cond.Status == corev1.ConditionTrue {
				return true, nil
			}
		}
	}

	return false, nil // Not ready yet
}

// addTolerationsToManifest processes a YAML manifest string,
// finds all Kubernetes Deployments and adds specified tolerations
// to their pod templates if they don't already exist.
func addTolerationsToManifest(manifestYAML string, ioStreams genericiooptions.IOStreams) (string, error) {
	decoder := k8syaml.NewYAMLOrJSONDecoder(strings.NewReader(manifestYAML), 4096)
	var resultBuilder strings.Builder
	firstDocument := true

	desiredTolerations := []corev1.Toleration{
		{
			Key:      "kubernetes.io/arch",
			Operator: corev1.TolerationOpEqual,
			Value:    "arm64",
			Effect:   corev1.TaintEffectNoSchedule,
		},
		{
			Key:      "kubernetes.io/arch",
			Operator: corev1.TolerationOpEqual,
			Value:    "amd64",
			Effect:   corev1.TaintEffectNoSchedule,
		},
	}

	for {
		var rawObj unstructured.Unstructured
		if err := decoder.Decode(&rawObj); err != nil {
			if err.Error() == "EOF" {
				break // End of YAML stream
			}
			_, _ = fmt.Fprintln(ioStreams.ErrOut, "Error decoding YAML document", "error", err)
			return "", fmt.Errorf("error decoding YAML document: %w", err)
		}

		if rawObj.Object == nil {
			continue // Skip empty documents
		}

		if !firstDocument {
			resultBuilder.WriteString("---\n")
		}
		firstDocument = false

		kind := rawObj.GetKind()
		apiVersion := rawObj.GetAPIVersion()

		if (apiVersion == "apps/v1" && kind == "Deployment") || (apiVersion == "apps/v1" && kind == "DaemonSet") {
			_, _ = fmt.Fprintln(ioStreams.Out, "Found resource, adding tolerations...",
				"kind", kind, "name", rawObj.GetName(), "namespace", rawObj.GetNamespace())

			spec, found, err := unstructured.NestedMap(rawObj.Object, "spec", "template", "spec")
			if err != nil {
				_, _ = fmt.Fprintln(ioStreams.ErrOut, "Error getting spec.template.spec", "error", err,
					"kind", kind, "name", rawObj.GetName())
				return "", fmt.Errorf("error getting spec.template.spec for %s %s: %w", kind, rawObj.GetName(), err)
			}
			if !found {
				_, _ = fmt.Fprintln(ioStreams.ErrOut, "spec.template.spec not found",
					"kind", kind, "name", rawObj.GetName())
				// If the path doesn't exist, we might not want to error out,
				// but rather just serialize the object as is.
				// However, for Deployments/DaemonSets, this path should exist.
				// For now, let's serialize as is and log.
				modifiedYAML, marshalErr := yaml.Marshal(rawObj.Object)
				if marshalErr != nil {
					return "", fmt.Errorf("error marshalling modified object: %w", marshalErr)
				}
				resultBuilder.Write(modifiedYAML)
				continue
			}

			tolerations, _, _ := unstructured.NestedSlice(spec, "tolerations")
			existingTolerations := make([]corev1.Toleration, 0, len(tolerations))
			for _, t := range tolerations {
				var tol corev1.Toleration
				if err := runtime.DefaultUnstructuredConverter.FromUnstructured(t.(map[string]any), &tol); err == nil {
					existingTolerations = append(existingTolerations, tol)
				}
			}

			newTolerations := existingTolerations
			for _, desiredTol := range desiredTolerations {
				exists := false
				for _, existingTol := range existingTolerations {
					if existingTol.Key == desiredTol.Key &&
						existingTol.Operator == desiredTol.Operator &&
						existingTol.Value == desiredTol.Value &&
						existingTol.Effect == desiredTol.Effect {
						exists = true
						break
					}
				}
				if !exists {
					newTolerations = append(newTolerations, desiredTol)
					_, _ = fmt.Fprintln(ioStreams.Out, "Adding toleration",
						"key", desiredTol.Key, "value", desiredTol.Value, "effect", desiredTol.Effect,
						"kind", kind, "name", rawObj.GetName())
				}
			}

			unstructuredTolerations := make([]any, len(newTolerations))
			for i, tol := range newTolerations {
				unstructuredTol, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&tol)
				if err != nil {
					return "", fmt.Errorf("error converting toleration to unstructured: %w", err)
				}
				unstructuredTolerations[i] = unstructuredTol
			}

			if err := unstructured.SetNestedSlice(spec, unstructuredTolerations, "tolerations"); err != nil {
				return "", fmt.Errorf("error setting tolerations: %w", err)
			}
			if err := unstructured.SetNestedMap(rawObj.Object, spec, "spec", "template", "spec"); err != nil {
				return "", fmt.Errorf("error setting updated spec: %w", err)
			}
		}

		modifiedYAML, err := yaml.Marshal(rawObj.Object)
		if err != nil {
			_, _ = fmt.Fprintln(ioStreams.ErrOut, "Error marshalling modified YAML", "error", err)
			return "", fmt.Errorf("error marshalling modified YAML: %w", err)
		}
		resultBuilder.Write(modifiedYAML)
	}

	return resultBuilder.String(), nil
}
