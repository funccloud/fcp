package knative

import (
	"bytes"
	"context"
	_ "embed" // Import the embed package
	"fmt"
	"text/template"
	"time"

	"github.com/go-logr/logr" // Import the kind package
	"go.funccloud.dev/fcp/internal/yamlutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:embed knative.yaml
var knativeServingYAML []byte // Embed the knative.yaml file

const (
	// Knative Operator version and URL
	knativeOperatorVersion = "v1.17.6"
	knativeOperatorURL     = "https://github.com/knative/operator/releases/download/knative-" +
		knativeOperatorVersion + "/operator.yaml"

	// Knative Operator details
	knativeOperatorNamespace  = "knative-operator"
	knativeOperatorDeployment = "knative-operator"

	// Knative Serving details (managed by Operator)
	knativeServingNamespace = "knative-serving" // Namespace where KnativeServing CR is created
	knativeServingCRName    = "knative-serving" // Name of the KnativeServing CR

	// Key Serving components for readiness check (managed by Operator)
	knativeServingController = "controller"
	knativeServingWebhook    = "webhook"

	applyTimeout  = 5 * time.Minute
	checkInterval = 10 * time.Second
	waitTimeout   = 15 * time.Minute
)

// InstallKnative installs Knative Serving using the Knative Operator.
func InstallKnative(ctx context.Context, domain, issuerName string, isKind bool, k8sClient client.Client, log logr.Logger) error {
	applyCtx, cancel := context.WithTimeout(ctx, applyTimeout)
	defer cancel()

	// 1. Apply Knative Operator manifest
	log.Info("Applying Knative Operator manifest...", "url", knativeOperatorURL)
	if err := yamlutil.ApplyManifestFromURL(applyCtx, k8sClient, log, knativeOperatorURL); err != nil {
		return fmt.Errorf("failed to apply Knative Operator manifest from %s: %w", knativeOperatorURL, err)
	}

	// 2. Wait for Knative Operator deployment to be ready
	operatorNN := types.NamespacedName{Namespace: knativeOperatorNamespace, Name: knativeOperatorDeployment}
	log.Info("Waiting for Knative Operator deployment to become ready...", "namespace", operatorNN.Namespace, "name", operatorNN.Name)
	waitCtxOperator, cancelOperatorWait := context.WithTimeout(ctx, waitTimeout)
	defer cancelOperatorWait()
	if err := waitForDeploymentReady(waitCtxOperator, k8sClient, operatorNN); err != nil {
		log.Error(err, "Knative Operator deployment did not become ready within timeout", "namespace", operatorNN.Namespace, "name", operatorNN.Name)
		return fmt.Errorf("knative operator deployment %s/%s did not become ready: %w", operatorNN.Namespace, operatorNN.Name, err)
	}
	log.Info("Knative Operator deployment is ready.", "namespace", operatorNN.Namespace, "name", operatorNN.Name)

	// 3. Ensure knative-serving namespace exists (Operator might not create it)
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: knativeServingNamespace}}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: knativeServingNamespace}, ns); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Creating knative-serving namespace")
			if createErr := k8sClient.Create(ctx, ns); createErr != nil {
				log.Error(createErr, "Failed to create knative-serving namespace")
				return fmt.Errorf("failed to create namespace %s: %w", knativeServingNamespace, createErr)
			}
		} else {
			log.Error(err, "Failed to check knative-serving namespace")
			return fmt.Errorf("failed to check namespace %s: %w", knativeServingNamespace, err)
		}
	}

	// 4. Apply KnativeServing CR from embedded YAML
	log.Info("Applying KnativeServing custom resource from embedded YAML...", "namespace", knativeServingNamespace, "name", knativeServingCRName)
	tpl, err := template.New("knativeServingTemplate").Parse(string(knativeServingYAML))
	if err != nil {
		log.Error(err, "Failed to parse embedded KnativeServing YAML template")
		return fmt.Errorf("failed to parse embedded KnativeServing YAML template: %w", err)
	}
	buff := bytes.Buffer{}
	err = tpl.Execute(&buff, map[string]any{
		"Domain":     domain,
		"IssuerName": issuerName,
		"IsKind":     isKind,
	})
	if err != nil {
		log.Error(err, "Failed to execute embedded KnativeServing YAML template")
		return fmt.Errorf("failed to execute embedded KnativeServing YAML template: %w", err)
	}
	// Decode the embedded YAML into an unstructured object
	knativeServingCR := &unstructured.Unstructured{}
	decoder := yaml.NewYAMLOrJSONDecoder(&buff, 4096)
	if err := decoder.Decode(knativeServingCR); err != nil {
		log.Error(err, "Failed to decode embedded KnativeServing YAML")
		return fmt.Errorf("failed to decode embedded KnativeServing YAML: %w", err)
	}

	// Ensure the namespace is set correctly (it should be in the YAML, but double-check)
	if knativeServingCR.GetNamespace() != knativeServingNamespace {
		log.Info("Setting namespace on embedded KnativeServing CR", "namespace", knativeServingNamespace)
		knativeServingCR.SetNamespace(knativeServingNamespace)
	}

	// Use Server-Side Apply (SSA)
	patch := client.Apply
	force := true
	patchOptions := &client.PatchOptions{FieldManager: "fcp-controller", Force: &force}
	if err = k8sClient.Patch(ctx, knativeServingCR, patch, patchOptions); err != nil { // Assign error to existing 'err' variable
		log.Error(err, "Failed to apply KnativeServing custom resource", "namespace", knativeServingNamespace, "name", knativeServingCRName)
		return fmt.Errorf("failed to apply KnativeServing CR %s/%s: %w", knativeServingNamespace, knativeServingCRName, err)
	}
	log.Info("KnativeServing custom resource applied.", "namespace", knativeServingNamespace, "name", knativeServingCRName)

	// 5. Wait for Knative Serving components (managed by Operator) to become ready
	log.Info("Waiting for Knative Serving components (managed by Operator) to become ready...")
	if err = waitForOperatorManagedDeploymentsReady(ctx, k8sClient, log); err != nil { // Assign error to existing 'err' variable
		return err // Error already logged in the function
	}

	// Note: patchKnativeNetworkConfig is no longer needed as Kourier is configured via KnativeServing CR

	log.Info("Knative Operator installation and Serving readiness checks completed.")
	return nil
}

// waitForOperatorManagedDeploymentsReady waits for the core Knative Serving deployments created by the Operator.
func waitForOperatorManagedDeploymentsReady(ctx context.Context, k8sClient client.Client, log logr.Logger) error {
	waitCtx, waitCancel := context.WithTimeout(ctx, waitTimeout)
	defer waitCancel()

	// Components expected to be created by the Operator in knative-serving namespace
	// Kourier deployment name might vary, add if needed after checking operator behavior
	components := []types.NamespacedName{
		{Namespace: knativeServingNamespace, Name: knativeServingController},
		{Namespace: knativeServingNamespace, Name: knativeServingWebhook},
		// Add Kourier deployment if its name is known and consistent, e.g.:
		// {Namespace: knativeServingNamespace, Name: "net-kourier-controller"},
	}

	for _, nn := range components {
		log.Info("Waiting for Operator-managed deployment...", "namespace", nn.Namespace, "name", nn.Name)
		if err := waitForDeploymentReady(waitCtx, k8sClient, nn); err != nil {
			// No alternative namespace check needed here as Operator manages these directly
			log.Error(err, "Operator-managed deployment did not become ready within timeout", "namespace", nn.Namespace, "name", nn.Name)
			return fmt.Errorf("operator-managed deployment %s/%s did not become ready: %w", nn.Namespace, nn.Name, err)
		}
		log.Info("Operator-managed deployment is ready", "namespace", nn.Namespace, "name", nn.Name)
	}

	// Additionally, wait for KnativeServing CR to report Ready status
	log.Info("Waiting for KnativeServing CR to become ready...", "namespace", knativeServingNamespace, "name", knativeServingCRName)
	knativeServingNN := types.NamespacedName{Namespace: knativeServingNamespace, Name: knativeServingCRName}
	if err := waitForKnativeServingReady(waitCtx, k8sClient, knativeServingNN); err != nil {
		log.Error(err, "KnativeServing CR did not become ready within timeout", "namespace", knativeServingNN.Namespace, "name", knativeServingNN.Name)
		return fmt.Errorf("KnativeServing CR %s/%s did not become ready: %w", knativeServingNN.Namespace, knativeServingNN.Name, err)
	}
	log.Info("KnativeServing CR is ready.", "namespace", knativeServingNN.Namespace, "name", knativeServingNN.Name)

	return nil
}

// waitForDeploymentReady waits for a deployment to have at least one available replica.
func waitForDeploymentReady(ctx context.Context, k8sClient client.Client, nn types.NamespacedName) error {
	return wait.PollUntilContextTimeout(ctx, checkInterval, waitTimeout, true, func(ctx context.Context) (bool, error) {
		dep := &appsv1.Deployment{}
		err := k8sClient.Get(ctx, nn, dep)
		if err != nil {
			if apierrors.IsNotFound(err) {
				// Deployment might not be created yet, keep polling
				return false, nil
			}
			return false, err // Other error, stop polling
		}

		// Check if the deployment is available
		for _, cond := range dep.Status.Conditions {
			if cond.Type == appsv1.DeploymentAvailable && cond.Status == "True" {
				return true, nil // Deployment is available
			}
		}
		// Not available yet
		return false, nil
	})
}

// waitForKnativeServingReady waits for the KnativeServing CR to report Ready=True.
func waitForKnativeServingReady(ctx context.Context, k8sClient client.Client, nn types.NamespacedName) error {
	return wait.PollUntilContextTimeout(ctx, checkInterval, waitTimeout, true, func(ctx context.Context) (bool, error) {
		ks := &unstructured.Unstructured{}
		ks.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "operator.knative.dev",
			Version: "v1beta1",
			Kind:    "KnativeServing",
		})

		err := k8sClient.Get(ctx, nn, ks)
		if err != nil {
			if apierrors.IsNotFound(err) {
				// KnativeServing CR might not be created yet or fully processed
				return false, nil
			}
			return false, fmt.Errorf("failed to get KnativeServing CR %s/%s: %w", nn.Namespace, nn.Name, err)
		}

		// Check status conditions
		conditions, found, err := unstructured.NestedSlice(ks.Object, "status", "conditions")
		if err != nil || !found {
			// Status or conditions not found yet, keep polling
			return false, nil
		}

		for _, c := range conditions {
			condition, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			condType, typeFound := condition["type"].(string)
			condStatus, statusFound := condition["status"].(string)

			if typeFound && statusFound && condType == "Ready" {
				return condStatus == string(metav1.ConditionTrue), nil // Return true if Ready=True
			}
		}

		// Ready condition not found yet
		return false, nil
	})
}
