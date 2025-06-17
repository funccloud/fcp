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

package workload

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	workloadv1alpha1 "go.funccloud.dev/fcp/api/workload/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
	servingv1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ = Describe("Application Controller", func() {
	const (
		AppName      = "test-app"
		AppNamespace = "default"
		AppImage     = "test-image:latest"
		AppDomain    = "test-app.example.com"
		timeout      = time.Second * 10
		interval     = time.Millisecond * 250
	)

	appKey := types.NamespacedName{Name: AppName, Namespace: AppNamespace}

	Context("When reconciling a basic Application", func() {
		var app *workloadv1alpha1.Application
		var cr ApplicationReconciler // Declare cr here

		BeforeEach(func() {
			// Initialize cr here, after k8sClient is set up
			cr = ApplicationReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			app = &workloadv1alpha1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      AppName,
					Namespace: AppNamespace,
				},
				Spec: workloadv1alpha1.ApplicationSpec{
					Containers: []corev1.Container{
						{
							Image: AppImage,
						},
					},
					Scale: workloadv1alpha1.Scale{ // Removed pointer
						MinReplicas: ptr.To[int32](1),
						MaxReplicas: ptr.To[int32](1),
					},
					RolloutDuration: &metav1.Duration{Duration: workloadv1alpha1.DefaultRolloutDuration},
					EnableTLS:       ptr.To(workloadv1alpha1.DefaultEnableTLS),
				},
			}
			err := k8sClient.Create(ctx, app)
			Expect(err).NotTo(HaveOccurred())
			// Use context with logger
			_, err = cr.Reconcile(ctx, ctrl.Request{NamespacedName: appKey})
			Expect(err).NotTo(HaveOccurred())
			// Use context with logger
			_, err = cr.Reconcile(ctx, ctrl.Request{NamespacedName: appKey})
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			// Clean up the application
			Expect(k8sClient.Delete(ctx, app)).Should(Succeed())
			// Use context with logger
			_, err := cr.Reconcile(ctx, ctrl.Request{NamespacedName: appKey})
			Expect(err).NotTo(HaveOccurred())
			// Wait for deletion
			Eventually(func() bool {
				err := k8sClient.Get(ctx, appKey, app)
				return apierrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
			ksvc := &servingv1.Service{ObjectMeta: metav1.ObjectMeta{Name: AppName, Namespace: AppNamespace}}
			_ = k8sClient.Delete(ctx, ksvc)
		})

		It("Should add the finalizer", func() {
			Eventually(func(g Gomega) {
				fetchedApp := &workloadv1alpha1.Application{}
				g.Expect(k8sClient.Get(ctx, appKey, fetchedApp)).Should(Succeed())
				g.Expect(controllerutil.ContainsFinalizer(fetchedApp, workloadv1alpha1.ApplicationFinalizer)).Should(BeTrue())
			}, timeout, interval).Should(Succeed())
		})

		It("Should create a Knative Service with correct spec and owner reference", func() {
			ksvcKey := types.NamespacedName{Name: AppName, Namespace: AppNamespace}
			Eventually(func(g Gomega) {
				ksvc := &servingv1.Service{}
				g.Expect(k8sClient.Get(ctx, ksvcKey, ksvc)).Should(Succeed())

				// Check Owner Reference
				g.Expect(ksvc.OwnerReferences).NotTo(BeEmpty())
				g.Expect(ksvc.OwnerReferences[0].APIVersion).Should(Equal(workloadv1alpha1.GroupVersion.String()))
				g.Expect(ksvc.OwnerReferences[0].Kind).Should(Equal("Application"))
				g.Expect(ksvc.OwnerReferences[0].Name).Should(Equal(AppName))
				g.Expect(ksvc.OwnerReferences[0].UID).Should(Equal(app.UID)) // Check UID after app is created

				// Check basic spec details
				g.Expect(ksvc.Spec.Template.Spec.Containers).NotTo(BeEmpty())
				g.Expect(ksvc.Spec.Template.Spec.Containers[0].Image).Should(Equal(AppImage))

				// Check labels
				g.Expect(ksvc.Labels[workloadv1alpha1.ApplicationLabel]).Should(Equal(AppName))
				g.Expect(ksvc.Spec.Template.Labels[workloadv1alpha1.ApplicationLabel]).Should(Equal(AppName))

			}, timeout, interval).Should(Succeed())
		})

		It("Should not create a DomainMapping if domain is not specified", func() {
			dmKey := types.NamespacedName{Name: AppDomain, Namespace: AppNamespace} // Use expected domain name
			Consistently(func(g Gomega) {
				dm := &servingv1beta1.DomainMapping{}
				err := k8sClient.Get(ctx, dmKey, dm)
				g.Expect(apierrors.IsNotFound(err)).Should(BeTrue())
			}, time.Second*2, interval).Should(Succeed()) // Check for a short period
		})

	})

	Context("When reconciling an Application with a Domain", func() {
		var app *workloadv1alpha1.Application
		var cr ApplicationReconciler // Declare cr here

		BeforeEach(func() {
			// Initialize cr here
			cr = ApplicationReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			app = &workloadv1alpha1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      AppName,
					Namespace: AppNamespace,
				},
				Spec: workloadv1alpha1.ApplicationSpec{
					Containers: []corev1.Container{
						{
							Image: AppImage,
						},
					},
					Domain: AppDomain,
					Scale: workloadv1alpha1.Scale{ // Removed pointer
						MinReplicas: ptr.To[int32](1),
						MaxReplicas: ptr.To[int32](1),
					},
					RolloutDuration: &metav1.Duration{Duration: workloadv1alpha1.DefaultRolloutDuration},
					EnableTLS:       ptr.To(workloadv1alpha1.DefaultEnableTLS),
				},
			}
			err := k8sClient.Create(ctx, app)
			Expect(err).NotTo(HaveOccurred())
			// Use context with logger
			_, err = cr.Reconcile(ctx, ctrl.Request{NamespacedName: appKey})
			Expect(err).NotTo(HaveOccurred())
			// Use context with logger
			_, err = cr.Reconcile(ctx, ctrl.Request{NamespacedName: appKey})
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			// Clean up the application
			Expect(k8sClient.Delete(ctx, app)).Should(Succeed())
			// Use context with logger
			_, err := cr.Reconcile(ctx, ctrl.Request{NamespacedName: appKey})
			Expect(err).NotTo(HaveOccurred())
			// Wait for deletion
			Eventually(func() bool {
				err := k8sClient.Get(ctx, appKey, app)
				return apierrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
			// Clean up potentially created resources (best effort)
			ksvc := &servingv1.Service{ObjectMeta: metav1.ObjectMeta{Name: AppName, Namespace: AppNamespace}}
			_ = k8sClient.Delete(ctx, ksvc)
			dm := &servingv1beta1.DomainMapping{ObjectMeta: metav1.ObjectMeta{Name: AppDomain, Namespace: AppNamespace}}
			_ = k8sClient.Delete(ctx, dm)
		})

		It("Should create a DomainMapping with correct spec and owner reference", func() {
			dmKey := types.NamespacedName{Name: AppDomain, Namespace: AppNamespace}
			Eventually(func(g Gomega) {
				dm := &servingv1beta1.DomainMapping{}
				g.Expect(k8sClient.Get(ctx, dmKey, dm)).Should(Succeed())

				// Check Owner Reference
				g.Expect(dm.OwnerReferences).NotTo(BeEmpty())
				g.Expect(dm.OwnerReferences[0].APIVersion).Should(Equal(workloadv1alpha1.GroupVersion.String()))
				g.Expect(dm.OwnerReferences[0].Kind).Should(Equal("Application"))
				g.Expect(dm.OwnerReferences[0].Name).Should(Equal(AppName))
				g.Expect(dm.OwnerReferences[0].UID).Should(Equal(app.UID)) // Check UID after app is created

				// Check Spec Ref
				g.Expect(dm.Spec.Ref.Kind).Should(Equal("Service"))
				g.Expect(dm.Spec.Ref.Namespace).Should(Equal(AppNamespace))
				g.Expect(dm.Spec.Ref.Name).Should(Equal(AppName))
				g.Expect(dm.Spec.Ref.APIVersion).Should(Equal(servingv1.SchemeGroupVersion.String()))

				// Check labels
				g.Expect(dm.Labels[workloadv1alpha1.ApplicationLabel]).Should(Equal(AppName))

			}, timeout, interval).Should(Succeed())
		})

	})

	Context("When deleting an Application", func() {
		var app *workloadv1alpha1.Application
		var cr ApplicationReconciler // Declare cr here

		BeforeEach(func() {
			// Initialize cr here
			cr = ApplicationReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			app = &workloadv1alpha1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      AppName,
					Namespace: AppNamespace,
				},
				Spec: workloadv1alpha1.ApplicationSpec{
					Containers: []corev1.Container{
						{
							Image: AppImage,
						},
					},
					Scale: workloadv1alpha1.Scale{ // Removed pointer
						MinReplicas: ptr.To[int32](1),
						MaxReplicas: ptr.To[int32](1),
					},
					RolloutDuration: &metav1.Duration{Duration: workloadv1alpha1.DefaultRolloutDuration},
					EnableTLS:       ptr.To(workloadv1alpha1.DefaultEnableTLS),
				},
			}
			err := k8sClient.Create(ctx, app)
			Expect(err).NotTo(HaveOccurred())
			// Use context with logger
			_, err = cr.Reconcile(ctx, ctrl.Request{NamespacedName: appKey})
			Expect(err).NotTo(HaveOccurred())
			// Use context with logger
			_, err = cr.Reconcile(ctx, ctrl.Request{NamespacedName: appKey})
			Expect(err).NotTo(HaveOccurred())
			// Ensure the app exists before deleting
			Eventually(func() error {
				return k8sClient.Get(ctx, appKey, &workloadv1alpha1.Application{})
			}, timeout, interval).Should(Succeed())

			// Initiate deletion
			Expect(k8sClient.Delete(ctx, app)).Should(Succeed())
			// Use context with logger
			_, err = cr.Reconcile(ctx, ctrl.Request{NamespacedName: appKey})
			Expect(err).NotTo(HaveOccurred())

		})

		// No AfterEach needed as the test verifies deletion

		It("Should remove the finalizer", func() {
			Eventually(func(g Gomega) {
				fetchedApp := &workloadv1alpha1.Application{}
				err := k8sClient.Get(ctx, appKey, fetchedApp)
				// Expect the app to be gone eventually after finalizer removal
				g.Expect(apierrors.IsNotFound(err)).Should(BeTrue())
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("When reconciling an Application that does not exist", func() {
		It("Should return nil and not error", func() {
			reconciler := &ApplicationReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			req := ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "non-existent-app", Namespace: AppNamespace},
			}
			// Use context with logger
			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result).Should(Equal(ctrl.Result{}))
		})
	})

	// Add more tests for specific annotation settings (TLS, scaling), status updates on errors, etc.
	Context("When reconciling Application with specific scaling annotations", func() {
		var app *workloadv1alpha1.Application
		var cr ApplicationReconciler // Declare cr here

		BeforeEach(func() {
			// Initialize cr here
			cr = ApplicationReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			app = &workloadv1alpha1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      AppName,
					Namespace: AppNamespace,
				},
				Spec: workloadv1alpha1.ApplicationSpec{
					Containers: []corev1.Container{
						{
							Image: AppImage,
						},
					},
					Scale: workloadv1alpha1.Scale{ // Removed pointer
						MinReplicas:                 ptr.To[int32](2),
						MaxReplicas:                 ptr.To[int32](5),
						Metric:                      workloadv1alpha1.MetricCPU,
						Target:                      ptr.To[int32](80), // Target is deprecated but let's test it
						TargetUtilizationPercentage: ptr.To[int32](75),
					},
					EnableTLS:       ptr.To(false), // Test disabling TLS
					RolloutDuration: &metav1.Duration{Duration: workloadv1alpha1.DefaultRolloutDuration},
				},
			}
			err := k8sClient.Create(ctx, app)
			Expect(err).NotTo(HaveOccurred())
			// Use context with logger
			_, err = cr.Reconcile(ctx, ctrl.Request{NamespacedName: appKey})
			Expect(err).NotTo(HaveOccurred())
			// Use context with logger
			_, err = cr.Reconcile(ctx, ctrl.Request{NamespacedName: appKey})
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(ctx, app)).Should(Succeed())
			// Use context with logger
			_, err := cr.Reconcile(ctx, ctrl.Request{NamespacedName: appKey})
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, appKey, app)
				return apierrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
			ksvc := &servingv1.Service{ObjectMeta: metav1.ObjectMeta{Name: AppName, Namespace: AppNamespace}}
			_ = k8sClient.Delete(ctx, ksvc)
		})

		It("Should set the correct annotations on the Knative Service", func() {
			ksvcKey := types.NamespacedName{Name: AppName, Namespace: AppNamespace}
			Eventually(func(g Gomega) {
				ksvc := &servingv1.Service{}
				g.Expect(k8sClient.Get(ctx, ksvcKey, ksvc)).Should(Succeed())
				g.Expect(ksvc.Annotations).To(HaveKeyWithValue("networking.knative.dev/disable-external-domain-tls", "true")) // TLS disabled
			}, timeout, interval).Should(Succeed())
		})
	})

})
