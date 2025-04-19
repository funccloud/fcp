package kind

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// IsKindCluster checks if the cluster is a Kind cluster by looking for the 'kindnet' DaemonSet.
func IsKindCluster(ctx context.Context, k8sClient client.Client) (bool, error) {
	ds := &appsv1.DaemonSet{}
	nn := types.NamespacedName{
		Namespace: "kube-system", // kindnetd runs in kube-system
		Name:      "kindnet",     // The name of the Kind network DaemonSet
	}

	err := k8sClient.Get(ctx, nn, ds)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// DaemonSet not found, likely not a Kind cluster
			fmt.Println("kindnet DaemonSet not found in kube-system.") // Consider using a logger
			return false, nil
		}
		// Other error occurred during the Get operation
		return false, fmt.Errorf("failed to get kindnet daemonset: %w", err)
	}

	// DaemonSet found, it's likely a Kind cluster
	fmt.Println("Found kindnet DaemonSet in kube-system.") // Consider using a logger
	return true, nil
}
