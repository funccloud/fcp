package resource

import (
	"context"

	"github.com/go-logr/logr"
	"go.funccloud.dev/fcp/internal/resource/certmanager"
	"go.funccloud.dev/fcp/internal/resource/kind"
	"go.funccloud.dev/fcp/internal/resource/knative"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CheckOrInstallVersion(ctx context.Context, domain string, k8sClient client.Client, log logr.Logger) error {
	log = log.WithName("ResourceCheckInstall")
	onKind, err := kind.IsKindCluster(ctx, k8sClient)
	if err != nil {
		log.Error(err, "Error checking for kindnet daemonset")
	}
	if onKind {
		log.Info("Detected Kind cluster via kindnet daemonset. Recommended for dev environment.")
		log.Info("Setting domain to 127.0.0.1.sslip.io")
		domain = "127.0.0.1.sslip.io"
	} else {
		log.Info("Did not detect Kind cluster (kindnet daemonset not found).")
	}

	// Check if cert-manager is installed
	// Subsequent assignments to err should use = as it's already declared
	err = certmanager.CheckOrInstallVersion(ctx, k8sClient, log)
	if err != nil {
		log.Error(err, "Error checking or installing cert-manager")
		return err
	}

	// Check if Knative is installed
	err = knative.CheckOrInstallVersion(ctx, domain, k8sClient, log)
	if err != nil {
		log.Error(err, "Error checking or installing Knative")
		return err
	}

	return nil
}
