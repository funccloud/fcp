package resource

import (
	"context"

	"github.com/go-logr/logr"
	"go.funccloud.dev/fcp/internal/resource/certmanager"
	"go.funccloud.dev/fcp/internal/resource/fcp"
	"go.funccloud.dev/fcp/internal/resource/kind"
	"go.funccloud.dev/fcp/internal/resource/knative"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CheckOrInstallVersion(ctx context.Context, domain string, k8sClient client.Client, log logr.Logger) error {
	log = log.WithName("ResourceCheckInstall")
	onKind, err := kind.IsKindCluster(ctx, k8sClient)
	if err != nil {
		log.Error(err, "Error checking for kindnet daemonset")
		// Decide if we should proceed or return; for now, assume not Kind if error occurs
		onKind = false
	}
	if onKind {
		log.Info("Detected Kind cluster via kindnet daemonset. Recommended for dev environment.")
		log.Info("Setting domain to 127.0.0.1.sslip.io")
		domain = "127.0.0.1.sslip.io"
	} else {
		log.Info("Did not detect Kind cluster (kindnet daemonset not found or error occurred).")
	}

	// Check if cert-manager is installed
	err = certmanager.CheckOrInstallVersion(ctx, k8sClient, log)
	if err != nil {
		log.Error(err, "Error checking or installing cert-manager")
		return err
	}

	// Check if Knative is installed, passing the onKind flag
	err = knative.CheckOrInstallVersion(ctx, domain, k8sClient, log, onKind) // Pass onKind here
	if err != nil {
		log.Error(err, "Error checking or installing Knative")
		return err
	}
	return nil
}

func SetupKubeAuthenticator(ctx context.Context, k8sClient client.Client, log logr.Logger) error {
	onKind, err := kind.IsKindCluster(ctx, k8sClient)
	if err != nil {
		log.Error(err, "Error checking for kindnet daemonset")
		// Decide if we should proceed or return; for now, assume not Kind if error occurs
		onKind = false
	}
	if onKind {
		err = fcp.SetupKubeAuthenticator(ctx, k8sClient, log, onKind)
		if err != nil {
			log.Error(err, "Error setting up kube-authenticator")
			return err
		}
	} else {
		log.Info("Did not detect Kind cluster (kindnet daemonset not found or error occurred).")
	}
	return nil
}
