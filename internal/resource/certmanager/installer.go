package certmanager

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	CertManagerVersion             = "v1.17.1"
	CertManagerManifestURLTemplate = "https://github.com/cert-manager/cert-manager/releases/download/%s/cert-manager.yaml"
	CertManagerCRDsURLTemplate     = "https://github.com/cert-manager/cert-manager/releases/download/%s/" +
		"cert-manager.crds.yaml"
)

// InstallCertManager attempts to install cert-manager by downloading its CRDs and main manifests and applying them.
func InstallCertManager(ctx context.Context, k8sClient client.Client, log logr.Logger) error {
	log = log.WithName("CertManagerInstall").WithValues("version", CertManagerVersion)
	log.Info("Cert-manager not found, attempting installation...")

	// 1. Install CRDs
	crdsURL := fmt.Sprintf(CertManagerCRDsURLTemplate, CertManagerVersion)
	log.Info("Downloading cert-manager CRDs manifest", "url", crdsURL)
	if err := applyManifestFromURL(ctx, k8sClient, log, crdsURL); err != nil {
		log.Error(err, "Failed to apply cert-manager CRDs manifest")
		return fmt.Errorf("failed to apply cert-manager CRDs from %s: %w", crdsURL, err)
	}
	log.Info("Cert-manager CRDs manifest applied successfully.")

	// Brief pause to allow CRDs to be established in the API server
	log.Info("Waiting briefly for CRDs to be established...")
	time.Sleep(10 * time.Second)

	// 2. Install main cert-manager components
	manifestURL := fmt.Sprintf(CertManagerManifestURLTemplate, CertManagerVersion)
	log.Info("Downloading main cert-manager manifest", "url", manifestURL)
	if err := applyManifestFromURL(ctx, k8sClient, log, manifestURL); err != nil {
		log.Error(err, "Failed to apply main cert-manager manifest")
		return fmt.Errorf("failed to apply main cert-manager manifest from %s: %w", manifestURL, err)
	}
	log.Info("Main cert-manager manifest applied successfully.")

	// 3. Wait for deployments to become ready
	log.Info("Waiting for cert-manager deployments to become ready...")
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Minute) // 5-minute timeout
	defer cancel()
	err := waitForCertManagerDeployments(waitCtx, k8sClient, log)
	if err != nil {
		log.Error(err, "Cert-manager deployments did not become ready in time")
		return fmt.Errorf("cert-manager deployments did not become ready: %w", err)
	}

	log.Info("Cert-manager installation completed successfully.")
	return nil
}

// applyManifestFromURL downloads a YAML manifest from a URL and applies its resources.
func applyManifestFromURL(ctx context.Context, k8sClient client.Client, log logr.Logger, url string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		log.Error(err, "Error creating HTTP request", "url", url)
		return fmt.Errorf("error creating request to download manifest: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Error(err, "Error downloading manifest", "url", url)
		return fmt.Errorf("error downloading manifest from %s: %w", url, err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			log.Error(cerr, "Error closing response body for manifest download", "url", url)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("status code %d", resp.StatusCode)
		log.Error(err, "Error downloading manifest", "url", url)
		return fmt.Errorf("non-OK status (%d) downloading manifest from %s", resp.StatusCode, url)
	}

	manifestBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error(err, "Error reading manifest response body", "url", url)
		return fmt.Errorf("error reading manifest: %w", err)
	}

	log.Info("Applying resources from manifest...", "url", url)
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(manifestBytes), 1024)
	var appliedCount int
	for {
		obj := &unstructured.Unstructured{}
		err := decoder.Decode(obj)
		if err == io.EOF {
			break // End of YAML file
		}
		if err != nil {
			log.Error(err, "Error decoding object from YAML manifest", "url", url)
			return fmt.Errorf("error decoding manifest: %w", err)
		}

		// Check if the decoded object's content is nil
		if obj.Object == nil {
			continue // Empty object, skip
		}

		log.Info("Applying resource...",
			"kind", obj.GetKind(), "name", obj.GetName(), "namespace", obj.GetNamespace())
		// Using Server Side Apply would be more robust, but Create/Update is simpler here.
		err = k8sClient.Create(ctx, obj)
		if err != nil {
			if apierrors.IsAlreadyExists(err) {
				log.Info("Resource already exists, attempting update...",
					"kind", obj.GetKind(), "name", obj.GetName(), "namespace", obj.GetNamespace())
				// Try to get the existing resource to fetch the resourceVersion
				existingObj := &unstructured.Unstructured{}
				existingObj.SetGroupVersionKind(obj.GroupVersionKind())
				getErr := k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), existingObj)
				if getErr == nil {
					obj.SetResourceVersion(existingObj.GetResourceVersion())
					updateErr := k8sClient.Update(ctx, obj)
					if updateErr != nil {
						log.Error(updateErr, "Failed to update existing resource",
							"kind", obj.GetKind(), "name", obj.GetName(), "namespace", obj.GetNamespace())
						// Do not return error here, could be an immutable resource or conflict
					} else {
						log.Info("Existing resource updated.",
							"kind", obj.GetKind(), "name", obj.GetName(), "namespace", obj.GetNamespace())
						appliedCount++
					}
				} else {
					log.Error(getErr, "Failed to get existing resource for update",
						"kind", obj.GetKind(), "name", obj.GetName(), "namespace", obj.GetNamespace())
				}
			} else {
				log.Error(err, "Error creating resource",
					"kind", obj.GetKind(), "name", obj.GetName(), "namespace", obj.GetNamespace())
				return fmt.Errorf("error creating resource %s/%s (%s): %w",
					obj.GetNamespace(), obj.GetName(), obj.GetKind(), err)
			}
		} else {
			appliedCount++
			log.Info("Resource created successfully.",
				"kind", obj.GetKind(), "name", obj.GetName(), "namespace", obj.GetNamespace())
		}

		// Short pause to avoid overwhelming the API server
		time.Sleep(50 * time.Millisecond)
	}

	log.Info("Manifest application finished.", "url", url, "resourcesCreatedOrUpdated", appliedCount)
	return nil
}

// waitForCertManagerDeployments waits for the main cert-manager deployments to be available.
func waitForCertManagerDeployments(ctx context.Context, k8sClient client.Client, log logr.Logger) error {
	deployments := []string{CertManagerDeployment, "cert-manager-webhook", "cert-manager-cainjector"}

	for _, depName := range deployments {
		log.Info("Waiting for deployment", "deployment", depName, "namespace", CertManagerNamespace)
		err := wait.PollUntilContextCancel(ctx, 5*time.Second, true, func(ctx context.Context) (bool, error) {
			ready, err := isDeploymentReady(ctx, k8sClient, CertManagerNamespace, depName)
			if err != nil {
				// If not found yet, keep waiting
				if apierrors.IsNotFound(err) {
					log.V(1).Info("Deployment not found yet, waiting...", "deployment", depName)
					return false, nil
				}
				log.Error(err, "Error checking deployment status", "deployment", depName)
				return false, err // Real error, stop waiting
			}
			if ready {
				log.Info("Deployment is ready", "deployment", depName)
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
