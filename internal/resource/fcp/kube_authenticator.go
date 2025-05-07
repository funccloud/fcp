package fcp

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	kubeAuthenticatorServiceName      = "fcp-kube-authenticator-webhook-service"
	kubeAuthenticatorServiceNamespace = "fcp-system"
)

func SetupKubeAuthenticator(ctx context.Context, k8sClient client.Client, l logr.Logger, onKind bool) error {
	if onKind {
		// Change service type to NodePort
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      kubeAuthenticatorServiceName,
				Namespace: kubeAuthenticatorServiceNamespace,
			},
			Spec: corev1.ServiceSpec{},
		}
		result, err := controllerutil.CreateOrUpdate(ctx, k8sClient, svc, func() error {
			svc.Spec.Type = corev1.ServiceTypeNodePort
			return nil
		})
		if err != nil {
			l.Error(err, "Failed to create or update kube-authenticator service")
			return err
		}
		if result != controllerutil.OperationResultNone {
			l.Info("Kube-authenticator service created or updated", "result", result)
		}
	}
	return nil
}
