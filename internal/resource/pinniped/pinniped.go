package pinniped

import (
	"context"

	"k8s.io/cli-runtime/pkg/genericiooptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CheckOrInstallVersion(ctx context.Context, k8sClient client.Client, domain, issuerName string, ioStreams genericiooptions.IOStreams) error {
	return nil
}
