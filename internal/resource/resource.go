package resource

import (
	"context"
	"fmt"

	"go.funccloud.dev/fcp/internal/resource/certmanager"
	"go.funccloud.dev/fcp/internal/resource/kind"
	"go.funccloud.dev/fcp/internal/resource/knative"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CheckOrInstallVersion(ctx context.Context, domain string, k8sClient client.Client, ioStreams genericiooptions.IOStreams) error {
	onKind, err := kind.IsKindCluster(ctx, k8sClient)
	if err != nil {
		_, _ = fmt.Fprintln(ioStreams.ErrOut, "Error checking for kindnet daemonset", "error", err)
		// Decide if we should proceed or return; for now, assume not Kind if error occurs
		onKind = false
	}
	if onKind {
		_, _ = fmt.Fprintln(ioStreams.Out, "Detected Kind cluster via kindnet daemonset. Recommended for dev environment.")
		if domain == "" {
			_, _ = fmt.Fprintln(ioStreams.Out, "Setting domain to 127.0.0.1.sslip.io")
			domain = "127.0.0.1.sslip.io"
		}
	} else {
		_, _ = fmt.Fprintln(ioStreams.Out, "Did not detect Kind cluster (kindnet daemonset not found or error occurred).")
	}

	// Check if cert-manager is installed
	err = certmanager.CheckOrInstallVersion(ctx, k8sClient, ioStreams)
	if err != nil {
		_, _ = fmt.Fprintln(ioStreams.ErrOut, "Error checking or installing cert-manager", "error", err)
		return err
	}

	// Check if Knative is installed, passing the onKind flag
	err = knative.CheckOrInstallVersion(ctx, domain, k8sClient, ioStreams, onKind) // Pass onKind here
	if err != nil {
		_, _ = fmt.Fprintln(ioStreams.ErrOut, "Error checking or installing Knative", "error", err)
		return err
	}

	return nil
}
