package certmanager

import (
	"context"
	"fmt"
	"time"

	"go.funccloud.dev/fcp/internal/yamlutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	if err := yamlutil.ApplyManifestFromURL(ctx, k8sClient, ioStreams, manifestURL); err != nil {
		_, _ = fmt.Fprintln(ioStreams.ErrOut, "Failed to apply main cert-manager manifest", "error", err)
		return fmt.Errorf("failed to apply main cert-manager manifest from %s: %w", manifestURL, err)
	}
	_, _ = fmt.Fprintln(ioStreams.Out, "Main cert-manager manifest applied successfully.")

	// 3. Wait for deployments to become ready
	_, _ = fmt.Fprintln(ioStreams.Out, "Waiting for cert-manager deployments to become ready...")
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Minute) // 5-minute timeout
	defer cancel()
	err := waitForCertManagerDeployments(waitCtx, k8sClient, ioStreams)
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
