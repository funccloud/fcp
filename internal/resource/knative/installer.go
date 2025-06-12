package knative

import (
	"bytes"
	"context"
	_ "embed" // Import the embed package
	"fmt"
	"io"
	"strings"
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
	"sigs.k8s.io/yaml"
)

//go:embed knative.yaml
var knativeServingYAML []byte // Embed the knative.yaml file

const (
	// Knative Operator version and URL
	knativeOperatorVersion = "v1.18.1"
	knativeVersion         = "v1.18.0"
	netContourVersion      = "v1.18.0"

	knativeOperatorURL = "https://github.com/knative/operator/releases/download/knative-" +
		knativeOperatorVersion + "/operator.yaml"

	// Knative Serving Default Domain URL (for Kind)
	knativeServingDefaultDomainURL = "https://github.com/knative/serving/releases/download/knative-" +
		knativeVersion + "/serving-default-domain.yaml"

	contourURL = "https://github.com/knative/net-contour/releases/download/knative-" +
		netContourVersion + "/contour.yaml"

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

	// Contour service modification details for Kind
	httpNodePort    = int64(31080)
	httpsNodePort   = int64(31443)
	httpTargetPort  = int64(8080) // Default target for HTTP
	httpsTargetPort = int64(8443) // Default target for HTTPS
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

	// 1. Fetch and Apply Contour manifest
	_, _ = fmt.Fprintln(ioStreams.Out, "Fetching Contour manifest from...", "url", contourURL)
	contourManifestBytes, err := yamlutil.DownloadYAMLFromURL(applyCtx, contourURL, ioStreams)
	if err != nil {
		return fmt.Errorf("failed to download Contour manifest from %s: %w", contourURL, err)
	}
	contourManifestContent := string(contourManifestBytes)

	// Add tolerations to Deployments and DaemonSets in the Contour manifest
	_, _ = fmt.Fprintln(ioStreams.Out, "Adding tolerations to Contour manifest...")
	modifiedContourManifestWithTolerations, err := addTolerationsToManifest(contourManifestContent, ioStreams)
	if err != nil {
		return fmt.Errorf("failed to add tolerations to Contour manifest: %w", err)
	}
	contourManifestContent = modifiedContourManifestWithTolerations

	if isKind {
		// The primary logging for this step is now within modifyContourServiceForKind
		modifiedManifest, err := modifyContourServiceForKind(contourManifestContent, ioStreams)
		if err != nil {
			return fmt.Errorf("failed to modify Contour manifest for Kind: %w", err)
		}
		contourManifestContent = modifiedManifest
	}

	_, _ = fmt.Fprintln(ioStreams.Out, "Applying Contour manifest...")
	if err := yamlutil.ApplyManifestYAML(applyCtx, k8sClient, contourManifestContent, ioStreams); err != nil {
		return fmt.Errorf("failed to apply Contour manifest: %w", err)
	}

	// 2. Apply Knative Operator manifest
	_, _ = fmt.Fprintln(ioStreams.Out, "Applying Knative Operator manifest...", "url", knativeOperatorURL)
	if err := yamlutil.ApplyManifestFromURL(applyCtx, k8sClient, ioStreams, knativeOperatorURL); err != nil {
		return fmt.Errorf("failed to apply Knative Operator manifest from %s: %w", knativeOperatorURL, err)
	}

	// 3. Wait for Knative Operator deployment to be ready
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

	// 4. Ensure knative-serving namespace exists (Operator might not create it)
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

	// 5. Apply KnativeServing CR from embedded YAML
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

// configureSpecForNodePort takes a service spec, modifies it to be a NodePort service
// with specific HTTP/HTTPS port configurations, and returns the modified spec.
// serviceNamespace and serviceName are used for logging purposes.
func configureSpecForNodePort(
	spec map[string]any,
	serviceNamespace, serviceName string,
	ioStreams genericiooptions.IOStreams,
) (map[string]any, error) {
	if err := unstructured.SetNestedField(spec, "NodePort", "type"); err != nil {
		return nil, fmt.Errorf("failed to set service type to NodePort for %s/%s: %w", serviceNamespace, serviceName, err)
	}

	ports, portsFound, errPorts := unstructured.NestedSlice(spec, "ports")
	if errPorts != nil {
		return nil, fmt.Errorf("error getting ports from service spec %s/%s: %w", serviceNamespace, serviceName, errPorts)
	}
	if !portsFound {
		ports = []any{} // Initialize if not found
	}

	var httpPortExists, httpsPortExists bool
	updatedPorts := []any{}

	for _, p := range ports {
		portMap, ok := p.(map[string]any)
		if !ok {
			updatedPorts = append(updatedPorts, p) // Keep non-map items
			continue
		}

		portNum, numFound, _ := unstructured.NestedInt64(portMap, "port")
		if !numFound {
			updatedPorts = append(updatedPorts, portMap) // Keep port if number not found
			continue
		}

		if portNum == 80 {
			_, _ = fmt.Fprintf(ioStreams.Out,
				"Updating HTTP port 80 for service %s/%s with NodePort %d and targetPort %d\\n",
				serviceNamespace, serviceName, httpNodePort, httpTargetPort) // nolint:errcheck
			if err := unstructured.SetNestedField(portMap, httpNodePort, "nodePort"); err != nil {
				return nil, fmt.Errorf("failed to set http nodePort for %s/%s: %w", serviceNamespace, serviceName, err)
			}
			// Ensure targetPort is set
			if err := unstructured.SetNestedField(portMap, httpTargetPort, "targetPort"); err != nil {
				return nil, fmt.Errorf("failed to set http targetPort for %s/%s: %w", serviceNamespace, serviceName, err)
			}
			httpPortExists = true
		} else if portNum == 443 {
			_, _ = fmt.Fprintf(ioStreams.Out,
				"Updating HTTPS port 443 for service %s/%s with NodePort %d and targetPort %d\\n",
				serviceNamespace, serviceName, httpsNodePort, httpsTargetPort) // nolint:errcheck
			if err := unstructured.SetNestedField(portMap, httpsNodePort, "nodePort"); err != nil {
				return nil, fmt.Errorf("failed to set https nodePort for %s/%s: %w", serviceNamespace, serviceName, err)
			}
			// Ensure targetPort is set
			if err := unstructured.SetNestedField(portMap, httpsTargetPort, "targetPort"); err != nil {
				return nil, fmt.Errorf("failed to set https targetPort for %s/%s: %w", serviceNamespace, serviceName, err)
			}
			httpsPortExists = true
		}
		updatedPorts = append(updatedPorts, portMap)
	}

	if !httpPortExists {
		_, _ = fmt.Fprintf(ioStreams.Out,
			"Adding HTTP port 80 for service %s/%s with NodePort %d and targetPort %d\\n",
			serviceNamespace, serviceName, httpNodePort, httpTargetPort) // nolint:errcheck
		updatedPorts = append(updatedPorts, map[string]any{
			"name": "http", "port": int64(80), "protocol": "TCP",
			"targetPort": httpTargetPort, "nodePort": httpNodePort,
		})
	}
	if !httpsPortExists {
		_, _ = fmt.Fprintf(ioStreams.Out,
			"Adding HTTPS port 443 for service %s/%s with NodePort %d and targetPort %d\\n",
			serviceNamespace, serviceName, httpsNodePort, httpsTargetPort) // nolint:errcheck
		updatedPorts = append(updatedPorts, map[string]any{
			"name": "https", "port": int64(443), "protocol": "TCP",
			"targetPort": httpsTargetPort, "nodePort": httpsNodePort,
		})
	}
	if err := unstructured.SetNestedSlice(spec, updatedPorts, "ports"); err != nil {
		return nil, fmt.Errorf("failed to set updated ports for %s/%s: %w", serviceNamespace, serviceName, err)
	}
	return spec, nil
}

// modifyLoadBalancerServicesToNodePort processes a YAML manifest string,
// finds all Kubernetes services of type LoadBalancer, changes their type to NodePort,
// and configures HTTP (80) and HTTPS (443) ports with specific nodePort and targetPort values.
func modifyLoadBalancerServicesToNodePort(manifestYAML string, ioStreams genericiooptions.IOStreams) (string, error) {
	decoder := k8syaml.NewYAMLOrJSONDecoder(strings.NewReader(manifestYAML), 4096)
	var resultBuilder strings.Builder
	firstDocument := true

	for {
		obj := &unstructured.Unstructured{}
		err := decoder.Decode(obj)
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("failed to decode YAML object: %w", err)
		}
		// Skip empty documents that might result from comments or multiple '---'
		if obj.Object == nil {
			continue
		}

		if obj.GetKind() == "Service" {
			spec, found, errSpec := unstructured.NestedMap(obj.Object, "spec")
			if errSpec != nil || !found {
				// Log error but continue processing other objects in the manifest.
				// This specific object will be marshaled as is, without modification attempts on its spec.
				_, _ = fmt.Fprintf(ioStreams.ErrOut, "Could not get spec for service %s/%s: %v. "+
					"Skipping modification for this object.\n", obj.GetNamespace(), obj.GetName(), errSpec)
			} else {
				serviceType, typeFound, _ := unstructured.NestedString(spec, "type")
				if typeFound && serviceType == "LoadBalancer" {
					_, _ = fmt.Fprintf(ioStreams.Out, "Changing service %s/%s type from LoadBalancer to NodePort\n",
						obj.GetNamespace(), obj.GetName())

					modifiedSpec, err := configureSpecForNodePort(spec, obj.GetNamespace(), obj.GetName(), ioStreams)
					if err != nil {
						return "", fmt.Errorf("failed to configure spec for NodePort for service %s/%s: %w",
							obj.GetNamespace(), obj.GetName(), err)
					}
					obj.Object["spec"] = modifiedSpec // Update the object's spec
				}
				// If not LoadBalancer, or type not found, do nothing to this service spec.
				// The object will be marshaled as is.
			}
		}

		modifiedYAMLBytes, err := yaml.Marshal(obj.Object)
		if err != nil {
			// If marshaling fails, it's a critical error for this object.
			return "", fmt.Errorf("failed to marshal object %s/%s to YAML: %w", obj.GetNamespace(), obj.GetName(), err)
		}
		if !firstDocument {
			resultBuilder.WriteString("---\n")
		}
		resultBuilder.Write(modifiedYAMLBytes)
		firstDocument = false
	}
	return resultBuilder.String(), nil
}

func modifyContourServiceForKind(manifestYAML string, ioStreams genericiooptions.IOStreams) (string, error) {
	_, _ = fmt.Fprintln(ioStreams.Out,
		"Modifying LoadBalancer services in Contour manifest to NodePort for Kind cluster...")
	return modifyLoadBalancerServicesToNodePort(manifestYAML, ioStreams)
}

// addTolerationsToManifest processes a YAML manifest string,
// finds all Kubernetes Deployments and DaemonSets, and adds specified tolerations
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
		obj := &unstructured.Unstructured{}
		err := decoder.Decode(obj)
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("failed to decode YAML object: %w", err)
		}
		if obj.Object == nil {
			continue
		}

		kind := obj.GetKind()
		if kind == "Deployment" || kind == "DaemonSet" {
			_, _ = fmt.Fprintf(ioStreams.Out, "Processing %s: %s/%s for tolerations\n", kind, obj.GetNamespace(), obj.GetName())

			// Tolerations for both Deployment and DaemonSet are under spec.template.spec.tolerations
			tolerationsPath := []string{"spec", "template", "spec", "tolerations"}
			currentTolerationsRaw, found, err := unstructured.NestedSlice(obj.Object, tolerationsPath...)

			if err != nil {
				_, _ = fmt.Fprintf(ioStreams.ErrOut, "Error getting tolerations for %s %s/%s: %v. Object will be marshaled without these toleration changes.\n", kind, obj.GetNamespace(), obj.GetName(), err)
				// Marshal the current object as is and continue to the next document,
				// effectively skipping the toleration modification logic for this object.
				modifiedYAMLBytes, marshalErr := yaml.Marshal(obj.Object)
				if marshalErr != nil {
					return "", fmt.Errorf("failed to marshal object %s/%s to YAML (after toleration fetch error): %w", obj.GetNamespace(), obj.GetName(), marshalErr)
				}
				if !firstDocument {
					resultBuilder.WriteString("---\n")
				}
				resultBuilder.Write(modifiedYAMLBytes)
				firstDocument = false
				continue // Continue to the next YAML document in the manifest
			}

			// If we reach here, err was nil. Proceed with toleration modification.
			var existingTolerations []corev1.Toleration
			if found {
				for _, tRaw := range currentTolerationsRaw {
					tolerationMap, ok := tRaw.(map[string]any)
					if !ok {
						_, _ = fmt.Fprintf(ioStreams.ErrOut, "Skipping non-map toleration item for %s %s/%s\\n", kind, obj.GetNamespace(), obj.GetName())
						continue
					}
					existingTolerations = append(existingTolerations, corev1.Toleration{
						Key:      fmt.Sprintf("%v", tolerationMap["key"]),
						Operator: corev1.TolerationOperator(fmt.Sprintf("%v", tolerationMap["operator"])),
						Value:    fmt.Sprintf("%v", tolerationMap["value"]),
						Effect:   corev1.TaintEffect(fmt.Sprintf("%v", tolerationMap["effect"])),
					})
				}
			}

			updatedTolerations := make([]any, len(currentTolerationsRaw))
			copy(updatedTolerations, currentTolerationsRaw)
			tolerationsModified := false

			for _, desired := range desiredTolerations {
				exists := false
				for _, existingRaw := range currentTolerationsRaw {
					existingMap, ok := existingRaw.(map[string]any)
					if !ok {
						continue
					}
					if existingMap["key"] == desired.Key &&
						existingMap["operator"] == string(desired.Operator) &&
						existingMap["value"] == desired.Value &&
						existingMap["effect"] == string(desired.Effect) {
						exists = true
						break
					}
				}

				if !exists {
					_, _ = fmt.Fprintf(ioStreams.Out, "Adding toleration %s:%s Op %s for %s %s/%s\\n", desired.Key, desired.Value, desired.Operator, kind, obj.GetNamespace(), obj.GetName())
					updatedTolerations = append(updatedTolerations, map[string]any{
						"key":      desired.Key,
						"operator": string(desired.Operator),
						"value":    desired.Value,
						"effect":   string(desired.Effect),
					})
					tolerationsModified = true
				}
			}

			if tolerationsModified {
				if errSet := unstructured.SetNestedSlice(obj.Object, updatedTolerations, tolerationsPath...); errSet != nil {
					_, _ = fmt.Fprintf(ioStreams.ErrOut, "Failed to set updated tolerations for %s %s/%s: %v. Skipping toleration modification for this object.\n", kind, obj.GetNamespace(), obj.GetName(), errSet)
				}
			}
		}

		modifiedYAMLBytes, err := yaml.Marshal(obj.Object)
		if err != nil {
			return "", fmt.Errorf("failed to marshal object %s/%s to YAML: %w", obj.GetNamespace(), obj.GetName(), err)
		}
		if !firstDocument {
			resultBuilder.WriteString("---\n")
		}
		resultBuilder.Write(modifiedYAMLBytes)
		firstDocument = false
	}
	return resultBuilder.String(), nil
}
