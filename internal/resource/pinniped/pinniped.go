package pinniped

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CheckOrInstallVersion checks if Pinniped is installed and installs it if not.
// It now uses the new InstallPinniped function from the installer.go file.
func CheckOrInstallVersion(ctx context.Context, k8sClient client.Client, domain, issuerName string, ioStreams genericiooptions.IOStreams, isKind bool) error {
	if issuerName == "" {
		issuerName = "le-prod-issuer"
		if isKind {
			issuerName = "le-staging-issuer"
		}
	}
	// Check for Pinniped Concierge deployment
	conciergeDeployment := &appsv1.Deployment{}
	conciergeNN := types.NamespacedName{
		Namespace: pinnipedConciergeNamespace,      // Uses const from installer.go
		Name:      pinnipedConciergeDeploymentName, // Uses const from installer.go
	}
	err := k8sClient.Get(ctx, conciergeNN, conciergeDeployment)
	if err != nil {
		if apierrors.IsNotFound(err) {
			_, _ = fmt.Fprintln(ioStreams.Out, "Pinniped Concierge not found, proceeding with installation.")
			// Pinniped not found, install it
			return InstallPinniped(ctx, k8sClient, domain, issuerName, ioStreams)
		}
		// Other error fetching deployment
		_, _ = fmt.Fprintln(ioStreams.ErrOut, "Failed to get Pinniped Concierge deployment:", err)
		return fmt.Errorf("failed to get Pinniped Concierge deployment %s/%s: %w", conciergeNN.Namespace, conciergeNN.Name, err)
	}

	// Check for Pinniped Supervisor deployment
	supervisorDeployment := &appsv1.Deployment{}
	supervisorNN := types.NamespacedName{
		Namespace: pinnipedSupervisorNamespace,      // Uses const from installer.go
		Name:      pinnipedSupervisorDeploymentName, // Uses const from installer.go
	}
	err = k8sClient.Get(ctx, supervisorNN, supervisorDeployment)
	if err != nil {
		if apierrors.IsNotFound(err) {
			_, _ = fmt.Fprintln(ioStreams.Out, "Pinniped Supervisor not found, proceeding with installation.")
			// Pinniped Supervisor not found, install it (InstallPinniped handles both)
			return InstallPinniped(ctx, k8sClient, domain, issuerName, ioStreams)
		}
		// Other error fetching deployment
		_, _ = fmt.Fprintln(ioStreams.ErrOut, "Failed to get Pinniped Supervisor deployment:", err)
		return fmt.Errorf("failed to get Pinniped Supervisor deployment %s/%s: %w", supervisorNN.Namespace, supervisorNN.Name, err)
	}

	_, _ = fmt.Fprintln(ioStreams.Out, "Pinniped Concierge and Supervisor are already installed.")
	return nil
}

// Constants are now defined in installer.go and are accessible within the package.
