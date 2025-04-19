package certmanager

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	CertManagerNamespace  = "cert-manager"
	CertManagerDeployment = "cert-manager"
)

// CheckOrInstallVersion checks if cert-manager is installed.
// If not installed, it attempts to install the version defined in installer.go.
// If installed, it checks if the version matches the expected one and logs a warning if different.
// Returns an error if the check fails or if installation is required and fails.
func CheckOrInstallVersion(ctx context.Context, k8sClient client.Client, log logr.Logger) error {
	log = log.WithName("CertManagerCheckInstall")

	deployment := &appsv1.Deployment{}
	namespacedName := types.NamespacedName{
		Namespace: CertManagerNamespace,
		Name:      CertManagerDeployment,
	}

	log.Info("Checking cert-manager installation...",
		"namespace", CertManagerNamespace, "deployment", CertManagerDeployment)

	err := k8sClient.Get(ctx, namespacedName, deployment)
	if err != nil {
		// Modified condition: Check for IsNotFound OR IsNoMatchError
		if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			log.Info("Cert-manager deployment or required CRDs not found. Attempting installation...")
			installErr := InstallCertManager(ctx, k8sClient, log) // Call the installation function
			if installErr != nil {
				log.Error(installErr, "Failed to install cert-manager")
				return fmt.Errorf("failed to install cert-manager: %w", installErr)
			}
			log.Info("Cert-manager installed successfully after check.")
			return nil // Successful installation
		}
		// Another error occurred while fetching the deployment
		log.Error(err, "Error fetching cert-manager deployment")
		return fmt.Errorf("error checking cert-manager: %w", err)
	}

	// Cert-manager is already installed, check the version (log only)
	log.Info("Cert-manager deployment found.", "namespace", CertManagerNamespace, "deployment", CertManagerDeployment)

	// Try to extract the version from the first container's image (usually the controller)
	foundVersion := ""
	if len(deployment.Spec.Template.Spec.Containers) > 0 {
		image := deployment.Spec.Template.Spec.Containers[0].Image
		parts := strings.Split(image, ":")
		if len(parts) > 1 {
			foundVersionTag := parts[len(parts)-1]
			// Remove 'v' prefix if present for comparison
			foundVersion = strings.TrimPrefix(foundVersionTag, "v")
		}
	}

	expectedVersionClean := strings.TrimPrefix(CertManagerVersion, "v")

	if foundVersion == "" {
		log.Info("Could not determine cert-manager version from container image.",
			"image", deployment.Spec.Template.Spec.Containers[0].Image,
			"reason", "Please check installation manually.")
	} else if foundVersion != expectedVersionClean {
		log.Info("WARNING: Found cert-manager version differs from expected.",
			"found", "v"+foundVersion, "expected", CertManagerVersion)
	} else {
		log.Info("Cert-manager version is correct.", "version", CertManagerVersion)
	}

	return nil // Check passed (cert-manager was already installed)
}
