/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package tenancy

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tenancyv1alpha1 "go.funccloud.dev/fcp/api/tenancy/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("Workspace Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name: resourceName,
		}
		workspace := &tenancyv1alpha1.Workspace{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind Workspace")
			err := k8sClient.Get(ctx, typeNamespacedName, workspace)
			if err != nil && errors.IsNotFound(err) {
				resource := &tenancyv1alpha1.Workspace{
					ObjectMeta: metav1.ObjectMeta{
						Name: resourceName,
					},
					TypeMeta: metav1.TypeMeta{
						Kind:       "Workspace",
						APIVersion: tenancyv1alpha1.GroupVersion.String(),
					},
					Spec: tenancyv1alpha1.WorkspaceSpec{
						Type: tenancyv1alpha1.WorkspaceTypePersonal,
						Owners: []corev1.ObjectReference{{
							Kind: "User",
							Name: "test-user",
						}},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})
		AfterEach(func() {
			resource := &tenancyv1alpha1.Workspace{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Workspace")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &WorkspaceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			By("Verifying the finalizer is set")
			// Fetch the resource again to check the finalizer
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, typeNamespacedName, workspace)).To(Succeed())
				g.Expect(workspace.Finalizers).To(ContainElement(tenancyv1alpha1.WorkspaceFinalizer))
			}, time.Minute, 10*time.Second).Should(Succeed())

			By("Verifying resources are created")
			// Run reconcile again after the finalizer is set
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(func(g Gomega) {
				ns := &corev1.Namespace{}
				nsName := types.NamespacedName{
					Name: workspace.Name,
				}
				g.Expect(k8sClient.Get(ctx, nsName, ns)).To(Succeed())
				role := &rbacv1.Role{}
				roleName := types.NamespacedName{
					Name:      workspace.Name,
					Namespace: ns.Name,
				}
				g.Expect(k8sClient.Get(ctx, roleName, role)).To(Succeed())
				roleBinding := &rbacv1.RoleBinding{}
				rolebindingName := types.NamespacedName{
					Name:      fmt.Sprintf("fcp-ownership-%s", workspace.Name),
					Namespace: workspace.Name,
				}
				g.Expect(k8sClient.Get(ctx, rolebindingName, roleBinding)).To(Succeed())
			}, time.Minute, 10*time.Second).Should(Succeed())
			By("Verfing istatus is set to ready")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, typeNamespacedName, workspace)).To(Succeed())
				cond := workspace.Status.GetCondition(tenancyv1alpha1.ReadyConditionType)
				g.Expect(cond).ToNot(BeNil())
				g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(cond.Reason).To(Equal(tenancyv1alpha1.RbacCreatedReason))
				g.Expect(cond.Message).To(Equal(fmt.Sprintf("Workspace %s is ready", workspace.Name)))
			}, time.Minute, 10*time.Second).Should(Succeed())
		})
	})
})
