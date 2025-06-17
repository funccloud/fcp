package knative

import (
	"bytes"
	"context"
	_ "embed" // Import the embed package
	"fmt"
	"text/template"
	"time"

	"go.funccloud.dev/fcp/internal/yamlutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:embed knative.yaml
var knativeServingYAML []byte // Embed the knative.yaml file

const (
	// Knative Operator version and URL
	knativeOperatorVersion = "v1.18.1"
	knativeVersion         = "v1.18.0"

	knativeOperatorURL = "https://github.com/knative/operator/releases/download/knative-" +
		knativeOperatorVersion + "/operator.yaml"

	// Knative Serving Default Domain URL (for Kind)
	knativeServingDefaultDomainURL = "https://github.com/knative/serving/releases/download/knative-" +
		knativeVersion + "/serving-default-domain.yaml"

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

	// Kong service modification details for Kind
)

// InstallKnative installs Knative Serving using the Knative Operator.
func InstallKnative(
	ctx context.Context,
	domain, issuerName string,
	isKind bool,
	k8sClient client.Client,
	ioStreams genericiooptions.IOStreams,
) error {
	applyCtx, cancel := context.WithTimeout(ctx, applyTimeout)
	defer cancel()

	// 1. Apply Knative Operator manifest
	_, _ = fmt.Fprintln(ioStreams.Out, "Applying Knative Operator manifest...", "url", knativeOperatorURL)
	if err := yamlutil.ApplyManifestFromURL(applyCtx, k8sClient, ioStreams, knativeOperatorURL); err != nil {
		return fmt.Errorf("failed to apply Knative Operator manifest from %s: %w", knativeOperatorURL, err)
	}

	// 2. Wait for Knative Operator deployment to be ready
	operatorNN := types.NamespacedName{Namespace: knativeOperatorNamespace, Name: knativeOperatorDeployment}
	_, _ = fmt.Fprintln(ioStreams.Out, "Waiting for Knative Operator deployment to become ready...",
		"namespace", operatorNN.Namespace, "name", operatorNN.Name)
	waitCtxOperator, cancelOperatorWait := context.WithTimeout(ctx, waitTimeout)
	defer cancelOperatorWait()
	if err := waitForDeploymentReady(waitCtxOperator, k8sClient, operatorNN); err != nil {
		_, _ = fmt.Fprintln(ioStreams.ErrOut, "Knative Operator deployment did not become ready within timeout",
			"namespace", operatorNN.Namespace, "name", operatorNN.Name, "error", err)
		return fmt.Errorf("knative operator deployment %s/%s did not become ready: %w",
			operatorNN.Namespace, operatorNN.Name, err)
	}
	_, _ = fmt.Fprintln(ioStreams.Out, "Knative Operator deployment is ready.",
		"namespace", operatorNN.Namespace, "name", operatorNN.Name)

	// 3. Ensure knative-serving namespace exists (Operator might not create it)
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: knativeServingNamespace}}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: knativeServingNamespace}, ns); err != nil {
		if apierrors.IsNotFound(err) {
			_, _ = fmt.Fprintln(ioStreams.Out, "Creating knative-serving namespace")
			if createErr := k8sClient.Create(ctx, ns); createErr != nil {
				_, _ = fmt.Fprintln(ioStreams.ErrOut, "Failed to create knative-serving namespace", "error", createErr)
				return fmt.Errorf("failed to create namespace %s: %w", knativeServingNamespace, createErr)
			}
		} else {
			_, _ = fmt.Fprintln(ioStreams.ErrOut, "Failed to check knative-serving namespace", "error", err)
			return fmt.Errorf("failed to check namespace %s: %w", knativeServingNamespace, err)
		}
	}

	// 4. Apply KnativeServing CR from embedded YAML
	_, _ = fmt.Fprintln(ioStreams.Out, "Applying KnativeServing custom resource from embedded YAML...",
		"namespace", knativeServingNamespace, "name", knativeServingCRName)
	tpl, err := template.New("knativeServingTemplate").Parse(string(knativeServingYAML))
	if err != nil {
		_, _ = fmt.Fprintln(ioStreams.ErrOut, "Failed to parse embedded KnativeServing YAML template", "error", err)
		return fmt.Errorf("failed to parse embedded KnativeServing YAML template: %w", err)
	}
	buff := bytes.Buffer{}
	err = tpl.Execute(&buff, map[string]any{
		"Domain":     domain,
		"IssuerName": issuerName,
		"IsKind":     isKind,
	})
	if err != nil {
		_, _ = fmt.Fprintln(ioStreams.ErrOut, "Failed to execute embedded KnativeServing YAML template", "error", err)
		return fmt.Errorf("failed to execute embedded KnativeServing YAML template: %w", err)
	}
	// Decode the embedded YAML into an unstructured object
	knativeServingCR := &unstructured.Unstructured{}
	// Use k8syaml for decoding from a reader
	decoder := k8syaml.NewYAMLOrJSONDecoder(&buff, 4096)
	if err := decoder.Decode(knativeServingCR); err != nil {
		_, _ = fmt.Fprintln(ioStreams.ErrOut, "Failed to decode embedded KnativeServing YAML", "error", err)
		return fmt.Errorf("failed to decode embedded KnativeServing YAML: %w", err)
	}

	// Ensure the namespace is set correctly (it should be in the YAML, but double-check)
	if knativeServingCR.GetNamespace() != knativeServingNamespace {
		_, _ = fmt.Fprintln(ioStreams.Out, "Setting namespace on embedded KnativeServing CR",
			"namespace", knativeServingNamespace)
		knativeServingCR.SetNamespace(knativeServingNamespace)
	}

	// Use Server-Side Apply (SSA)
	patch := client.Apply
	force := true
	patchOptions := &client.PatchOptions{FieldManager: "fcp-controller", Force: &force}
	// Assign error to existing 'err' variable
	if err = k8sClient.Patch(ctx, knativeServingCR, patch, patchOptions); err != nil {
		_, _ = fmt.Fprintln(ioStreams.ErrOut, "Failed to apply KnativeServing custom resource",
			"namespace", knativeServingNamespace, "name", knativeServingCRName, "error", err)
		return fmt.Errorf("failed to apply KnativeServing CR %s/%s: %w",
			knativeServingNamespace, knativeServingCRName, err)
	}
	_, _ = fmt.Fprintln(ioStreams.Out, "KnativeServing custom resource applied.",
		"namespace", knativeServingNamespace, "name", knativeServingCRName)

	// 6. Wait for Knative Serving components (managed by Operator) to become ready (Step was misnumbered as 5 before)
	_, _ = fmt.Fprintln(ioStreams.Out, "Waiting for Knative Serving components (managed by Operator) to become ready...")
	// Assign error to existing 'err' variable
	if err = waitForOperatorManagedDeploymentsReady(ctx, k8sClient, ioStreams); err != nil {
		return err // Error already logged in the function
	}
	if isKind {
		_, _ = fmt.Fprintln(ioStreams.Out, "Applying Knative Serving default domain manifest for Kind...",
			"url", knativeServingDefaultDomainURL)
		if err := yamlutil.ApplyManifestFromURL(applyCtx, k8sClient, ioStreams, knativeServingDefaultDomainURL); err != nil {
			_, _ = fmt.Fprintln(ioStreams.ErrOut, "Failed to apply Knative Serving default domain manifest",
				"url", knativeServingDefaultDomainURL, "error", err)
			return fmt.Errorf("failed to apply Knative Serving default domain manifest from %s: %w",
				knativeServingDefaultDomainURL, err)
		} else {
			_, _ = fmt.Fprintln(ioStreams.Out, "Successfully applied Knative Serving default domain manifest.")
		}
	}
	_, _ = fmt.Fprintln(ioStreams.Out, "Knative Operator installation and Serving readiness checks completed.")
	return nil
}

// waitForOperatorManagedDeploymentsReady waits for the core Knative Serving deployments created by the Operator.
func waitForOperatorManagedDeploymentsReady(
	ctx context.Context,
	k8sClient client.Client,
	ioStreams genericiooptions.IOStreams,
) error {
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
		_, _ = fmt.Fprintln(ioStreams.Out, "Waiting for Operator-managed deployment...",
			"namespace", nn.Namespace, "name", nn.Name)
		if err := waitForDeploymentReady(waitCtx, k8sClient, nn); err != nil {
			// No alternative namespace check needed here as Operator manages these directly
			_, _ = fmt.Fprintln(ioStreams.ErrOut, "Operator-managed deployment did not become ready within timeout",
				"namespace", nn.Namespace, "name", nn.Name, "error", err)
			return fmt.Errorf("operator-managed deployment %s/%s did not become ready: %w",
				nn.Namespace, nn.Name, err)
		}
		_, _ = fmt.Fprintln(ioStreams.Out, "Operator-managed deployment is ready",
			"namespace", nn.Namespace, "name", nn.Name)
	}

	// Additionally, wait for KnativeServing CR to report Ready status
	_, _ = fmt.Fprintln(ioStreams.Out, "Waiting for KnativeServing CR to become ready...",
		"namespace", knativeServingNamespace, "name", knativeServingCRName)
	knativeServingNN := types.NamespacedName{Namespace: knativeServingNamespace, Name: knativeServingCRName}
	if err := waitForKnativeServingReady(waitCtx, k8sClient, knativeServingNN); err != nil {
		_, _ = fmt.Fprintln(ioStreams.ErrOut, "KnativeServing CR did not become ready within timeout",
			"namespace", knativeServingNN.Namespace, "name", knativeServingNN.Name, "error", err)
		return fmt.Errorf("KnativeServing CR %s/%s did not become ready: %w",
			knativeServingNN.Namespace, knativeServingNN.Name, err)
	}
	_, _ = fmt.Fprintln(ioStreams.Out, "KnativeServing CR is ready.",
		"namespace", knativeServingNN.Namespace, "name", knativeServingNN.Name)

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
