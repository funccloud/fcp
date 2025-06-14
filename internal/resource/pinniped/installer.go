package pinniped

import (
	"bytes"
	"context"
	_ "embed" // Required for go:embed
	"fmt"
	"text/template"
	"time"

	"go.funccloud.dev/fcp/internal/yamlutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:embed pinniped-concierge.yaml
var conciergeManifestYAML string

//go:embed pinniped-supervisor.yaml
var supervisorManifestYAML string

const (
	pinnipedConciergeNamespace  = "pinniped-concierge"
	pinnipedSupervisorNamespace = "pinniped-supervisor"

	pinnipedConciergeDeploymentName  = "pinniped-concierge"  // Common name, verify if different
	pinnipedSupervisorDeploymentName = "pinniped-supervisor" // Confirmed from user-provided YAML

	applyTimeout  = 5 * time.Minute
	checkInterval = 10 * time.Second
	waitTimeout   = 15 * time.Minute
)

// InstallPinniped installs Pinniped Concierge and Supervisor components.
// It follows a similar pattern to knative/InstallKnative.
func InstallPinniped(
	ctx context.Context,
	k8sClient client.Client,
	domain string,
	issuerName string,
	ioStreams genericiooptions.IOStreams,
) error {
	_, _ = fmt.Fprintln(ioStreams.Out, "Starting Pinniped installation...")

	templateData := map[string]string{
		"Domain":     domain,
		"IssuerName": issuerName,
	}

	// Ensure namespaces exist
	for _, nsName := range []string{pinnipedConciergeNamespace, pinnipedSupervisorNamespace} {
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, ns); err != nil {
			if apierrors.IsNotFound(err) {
				_, _ = fmt.Fprintln(ioStreams.Out, "Creating namespace:", nsName)
				if createErr := k8sClient.Create(ctx, ns); createErr != nil {
					_, _ = fmt.Fprintln(ioStreams.ErrOut, "Failed to create namespace", nsName, ":", createErr)
					return fmt.Errorf("failed to create namespace %s: %w", nsName, createErr)
				}
			} else {
				_, _ = fmt.Fprintln(ioStreams.ErrOut, "Failed to check namespace", nsName, ":", err)
				return fmt.Errorf("failed to check namespace %s: %w", nsName, err)
			}
		}
	}

	// 1. Install Pinniped Concierge
	_, _ = fmt.Fprintln(ioStreams.Out, "Applying Pinniped Concierge manifest...")
	conciergeManifestContent, err := processTemplate("pinniped-concierge", conciergeManifestYAML, templateData)
	if err != nil {
		_, _ = fmt.Fprintln(ioStreams.ErrOut, "Failed to process Pinniped Concierge template:", err)
		return fmt.Errorf("failed to process Pinniped Concierge template: %w", err)
	}

	applyCtxConcierge, cancelConciergeApply := context.WithTimeout(ctx, applyTimeout)
	defer cancelConciergeApply()
	if err := yamlutil.ApplyManifestYAML(applyCtxConcierge, k8sClient, conciergeManifestContent, ioStreams); err != nil {
		_, _ = fmt.Fprintln(ioStreams.ErrOut, "Failed to apply Pinniped Concierge manifest:", err)
		return fmt.Errorf("failed to apply Pinniped Concierge manifest: %w", err)
	}
	_, _ = fmt.Fprintln(ioStreams.Out, "Pinniped Concierge manifest applied.")

	conciergeNN := types.NamespacedName{Namespace: pinnipedConciergeNamespace, Name: pinnipedConciergeDeploymentName}
	_, _ = fmt.Fprintln(ioStreams.Out, "Waiting for Pinniped Concierge deployment to become ready...",
		"namespace", conciergeNN.Namespace, "name", conciergeNN.Name)
	waitCtxConcierge, cancelConciergeWait := context.WithTimeout(ctx, waitTimeout)
	defer cancelConciergeWait()
	if err := waitForDeploymentReady(waitCtxConcierge, k8sClient, conciergeNN); err != nil {
		_, _ = fmt.Fprintln(ioStreams.ErrOut, "Pinniped Concierge deployment did not become ready within timeout.",
			"namespace", conciergeNN.Namespace, "name", conciergeNN.Name, "error", err)
		return fmt.Errorf("Pinniped Concierge deployment %s/%s did not become ready: %w",
			conciergeNN.Namespace, conciergeNN.Name, err)
	}
	_, _ = fmt.Fprintln(ioStreams.Out, "Pinniped Concierge deployment is ready.",
		"namespace", conciergeNN.Namespace, "name", conciergeNN.Name)

	// 2. Install Pinniped Supervisor
	_, _ = fmt.Fprintln(ioStreams.Out, "Applying Pinniped Supervisor manifest...")
	supervisorManifestContent, err := processTemplate("pinniped-supervisor", supervisorManifestYAML, templateData)
	if err != nil {
		_, _ = fmt.Fprintln(ioStreams.ErrOut, "Failed to process Pinniped Supervisor template:", err)
		return fmt.Errorf("failed to process Pinniped Supervisor template: %w", err)
	}

	applyCtxSupervisor, cancelSupervisorApply := context.WithTimeout(ctx, applyTimeout)
	defer cancelSupervisorApply()
	if err := yamlutil.ApplyManifestYAML(applyCtxSupervisor, k8sClient, supervisorManifestContent, ioStreams); err != nil {
		_, _ = fmt.Fprintln(ioStreams.ErrOut, "Failed to apply Pinniped Supervisor manifest:", err)
		return fmt.Errorf("failed to apply Pinniped Supervisor manifest: %w", err)
	}
	_, _ = fmt.Fprintln(ioStreams.Out, "Pinniped Supervisor manifest applied.")

	supervisorNN := types.NamespacedName{Namespace: pinnipedSupervisorNamespace, Name: pinnipedSupervisorDeploymentName}
	_, _ = fmt.Fprintln(ioStreams.Out, "Waiting for Pinniped Supervisor deployment to become ready...",
		"namespace", supervisorNN.Namespace, "name", supervisorNN.Name)
	waitCtxSupervisor, cancelSupervisorWait := context.WithTimeout(ctx, waitTimeout)
	defer cancelSupervisorWait()
	if err := waitForDeploymentReady(waitCtxSupervisor, k8sClient, supervisorNN); err != nil {
		_, _ = fmt.Fprintln(ioStreams.ErrOut, "Pinniped Supervisor deployment did not become ready within timeout.",
			"namespace", supervisorNN.Namespace, "name", supervisorNN.Name, "error", err)
		return fmt.Errorf("Pinniped Supervisor deployment %s/%s did not become ready: %w",
			supervisorNN.Namespace, supervisorNN.Name, err)
	}
	_, _ = fmt.Fprintln(ioStreams.Out, "Pinniped Supervisor deployment is ready.",
		"namespace", supervisorNN.Namespace, "name", supervisorNN.Name)

	_, _ = fmt.Fprintln(ioStreams.Out, "Pinniped installation completed successfully.")
	return nil
}

// processTemplate executes a text template with the given data.
func processTemplate(templateName, manifestYAML string, data interface{}) (string, error) {
	tpl, err := template.New(templateName).Parse(manifestYAML)
	if err != nil {
		return "", fmt.Errorf("failed to parse template %s: %w", templateName, err)
	}
	var buff bytes.Buffer
	if err := tpl.Execute(&buff, data); err != nil {
		return "", fmt.Errorf("failed to execute template %s: %w", templateName, err)
	}
	return buff.String(), nil
}

// waitForDeploymentReady waits for a deployment to have at least one available replica.
// This function is similar to the one in knative/installer.go.
func waitForDeploymentReady(
	ctx context.Context,
	k8sClient client.Client,
	nn types.NamespacedName,
) error {
	// Shorten the line by moving the function literal to a new line
	return wait.PollUntilContextTimeout(ctx, checkInterval, waitTimeout, true,
		func(ctxPoll context.Context) (bool, error) {
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctxPoll, nn, dep)
			if err != nil {
				if apierrors.IsNotFound(err) {
					return false, nil // Keep polling
				}
				return false, err // Stop polling on other errors
			}

			if dep.Status.ObservedGeneration < dep.Generation {
				return false, nil // Wait for status to catch up to spec
			}

			for _, cond := range dep.Status.Conditions {
				if cond.Type == appsv1.DeploymentAvailable && cond.Status == corev1.ConditionTrue {
					return true, nil // Deployment is available
				}
			}
			return false, nil // Not available yet
		},
	)
}
