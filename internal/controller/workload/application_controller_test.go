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
	"knative.dev/pkg/apis" // For apis.ParseURL
	duckv1 "knative.dev/pkg/apis/duck/v1"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"
	servingv1beta1 "knative.dev/serving/pkg/apis/serving/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"k8s.io/apimachinery/pkg/api/meta" // For meta.FindStatusCondition
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

				g.Expect(ksvc.Spec.Template.Annotations).To(HaveKeyWithValue("autoscaling.knative.dev/min-scale", "2"))
				g.Expect(ksvc.Spec.Template.Annotations).To(HaveKeyWithValue("autoscaling.knative.dev/max-scale", "5"))
				g.Expect(ksvc.Spec.Template.Annotations).To(HaveKeyWithValue("networking.knative.dev/disable-external-domain-tls", "true"))

			}, timeout, interval).Should(Succeed())
		})
	})

	Context("When handling Application status based on Knative Service readiness", func() {
		var app *workloadv1alpha1.Application
		var cr ApplicationReconciler
		var ksvc *servingv1.Service

		BeforeEach(func() {
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
					Containers: []corev1.Container{{Image: AppImage}},
					Scale:      workloadv1alpha1.Scale{MinReplicas: ptr.To[int32](1), MaxReplicas: ptr.To[int32](1)},
				},
			}
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())

			// Initial reconcile to create the Knative Service
			_, err := cr.Reconcile(ctx, ctrl.Request{NamespacedName: appKey})
			Expect(err).NotTo(HaveOccurred())

			ksvc = &servingv1.Service{}
			Eventually(func() error {
				return k8sClient.Get(ctx, appKey, ksvc)
			}, timeout, interval).Should(Succeed(), "Knative Service should be created by initial reconcile")
		})

		AfterEach(func() {
			if ksvc != nil {
				Expect(k8sClient.Delete(ctx, ksvc)).Should(Succeed())
			}
			if app != nil {
				Expect(k8sClient.Delete(ctx, app)).Should(Succeed())
				Eventually(func() bool {
					err := k8sClient.Get(ctx, appKey, &workloadv1alpha1.Application{})
					return apierrors.IsNotFound(err)
				}, timeout, interval).Should(BeTrue())
			}
		})

		It("Should set Application Ready when Knative Service becomes Ready", func() {
			// Simulate Knative Service becoming ready
			ksvc.Status.SetConditions(duckv1.Conditions{{
				Type:    servingv1.ServiceConditionReady,
				Status:  corev1.ConditionTrue,
				Reason:  "KsvcReadyForTest",
				Message: "Knative service is ready for test",
			}})
			ksvc.Status.URL, _ = apis.ParseURL("http://" + AppName + "." + AppNamespace + ".example.com")
			ksvc.Status.ObservedGeneration = ksvc.Generation
			Expect(k8sClient.Status().Update(ctx, ksvc)).Should(Succeed())

			// Reconcile Application again
			_, err := cr.Reconcile(ctx, ctrl.Request{NamespacedName: appKey})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func(g Gomega) {
				fetchedApp := &workloadv1alpha1.Application{}
				g.Expect(k8sClient.Get(ctx, appKey, fetchedApp)).Should(Succeed())
				readyCond := meta.FindStatusCondition(fetchedApp.Status.Conditions, workloadv1alpha1.ReadyConditionType)
				g.Expect(readyCond).NotTo(BeNil())
				g.Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(readyCond.Reason).To(Equal(workloadv1alpha1.ResourcesCreatedReason)) // This is the final ready reason
			}, timeout, interval).Should(Succeed())
		})

		It("Should set Application NotReady with specific message when Knative Service is NotReady", func() {
			// Simulate Knative Service being not ready with a specific message
			ksvc.Status.SetConditions(duckv1.Conditions{{
				Type:    servingv1.ServiceConditionReady,
				Status:  corev1.ConditionFalse,
				Reason:  "KsvcDeployFailedForTest",
				Message: "Knative deployment failed for test",
			}})
			ksvc.Status.ObservedGeneration = ksvc.Generation
			Expect(k8sClient.Status().Update(ctx, ksvc)).Should(Succeed())

			// Reconcile Application
			_, err := cr.Reconcile(ctx, ctrl.Request{NamespacedName: appKey})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func(g Gomega) {
				fetchedApp := &workloadv1alpha1.Application{}
				g.Expect(k8sClient.Get(ctx, appKey, fetchedApp)).Should(Succeed())
				readyCond := meta.FindStatusCondition(fetchedApp.Status.Conditions, workloadv1alpha1.ReadyConditionType)
				g.Expect(readyCond).NotTo(BeNil())
				g.Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(readyCond.Reason).To(Equal(workloadv1alpha1.KnativeServiceNotReadyReason))
				g.Expect(readyCond.Message).To(Equal("Knative deployment failed for test"))

				ksvcReadyCond := meta.FindStatusCondition(fetchedApp.Status.Conditions, workloadv1alpha1.KnativeServiceReadyConditionType)
				g.Expect(ksvcReadyCond).NotTo(BeNil())
				g.Expect(ksvcReadyCond.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(ksvcReadyCond.Reason).To(Equal(workloadv1alpha1.KnativeServiceNotReadyReason))
				g.Expect(ksvcReadyCond.Message).To(Equal("Knative deployment failed for test"))
			}, timeout, interval).Should(Succeed())
		})

		It("Should set Application NotReady with generic message when Knative Service Ready condition is missing", func() {
			// Simulate Knative Service with a missing Ready condition (e.g., only other conditions present)
			ksvc.Status.SetConditions(duckv1.Conditions{{
				Type:   servingv1.ServiceConditionConfigurationsReady, // A different, non-Ready condition
				Status: corev1.ConditionTrue,
			}})
			// Or ksvc.Status.Conditions = duckv1.Conditions{} // to simulate completely missing
			ksvc.Status.ObservedGeneration = ksvc.Generation
			Expect(k8sClient.Status().Update(ctx, ksvc)).Should(Succeed())

			// Reconcile Application
			_, err := cr.Reconcile(ctx, ctrl.Request{NamespacedName: appKey})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func(g Gomega) {
				fetchedApp := &workloadv1alpha1.Application{}
				g.Expect(k8sClient.Get(ctx, appKey, fetchedApp)).Should(Succeed())
				readyCond := meta.FindStatusCondition(fetchedApp.Status.Conditions, workloadv1alpha1.ReadyConditionType)
				g.Expect(readyCond).NotTo(BeNil())
				g.Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(readyCond.Reason).To(Equal(workloadv1alpha1.KnativeServiceNotReadyReason))
				g.Expect(readyCond.Message).To(Equal("Knative Service Ready condition is missing."))

				ksvcReadyCond := meta.FindStatusCondition(fetchedApp.Status.Conditions, workloadv1alpha1.KnativeServiceReadyConditionType)
				g.Expect(ksvcReadyCond).NotTo(BeNil())
				g.Expect(ksvcReadyCond.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(ksvcReadyCond.Reason).To(Equal(workloadv1alpha1.KnativeServiceNotReadyReason))
				g.Expect(ksvcReadyCond.Message).To(Equal("Knative Service Ready condition is missing."))
			}, timeout, interval).Should(Succeed())
		})
	})
})
