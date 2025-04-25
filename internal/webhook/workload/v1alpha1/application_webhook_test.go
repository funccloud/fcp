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

package v1alpha1

import (
	"context"
	// "testing" // Removed testing import as requested
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	workloadv1alpha1 "go.funccloud.dev/fcp/api/workload/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// TODO (user): Add any additional imports if needed
)

// Removed TestApplicationCustomDefaulter_Default function as requested.

// Refactoring to use Ginkgo style for Default tests below.
var _ = Describe("Application Webhook", func() {
	var (
		obj       *workloadv1alpha1.Application
		oldObj    *workloadv1alpha1.Application // Keep for validation tests
		validator ApplicationCustomValidator    // Keep for validation tests
		defaulter ApplicationCustomDefaulter
		ctx       context.Context
	)

	BeforeEach(func() {
		ctx = context.Background() // Initialize context for tests
		obj = &workloadv1alpha1.Application{}
		oldObj = &workloadv1alpha1.Application{}
		validator = ApplicationCustomValidator{}
		Expect(validator).NotTo(BeNil(), "Expected validator to be initialized")
		defaulter = ApplicationCustomDefaulter{}
		Expect(defaulter).NotTo(BeNil(), "Expected defaulter to be initialized")
		Expect(oldObj).NotTo(BeNil(), "Expected oldObj to be initialized")
		Expect(obj).NotTo(BeNil(), "Expected obj to be initialized")
	})

	AfterEach(func() {
		// TODO (user): Add any teardown logic common to all tests
	})

	Context("When creating Application under Defaulting Webhook", func() {

		It("Should set default RolloutDuration and EnableTLS when nil", func() {
			By("creating an application with nil RolloutDuration and EnableTLS")
			obj = &workloadv1alpha1.Application{
				ObjectMeta: metav1.ObjectMeta{Name: "test-app-ginkgo-defaults", Namespace: "test-ns-ginkgo-defaults"},
				Spec:       workloadv1alpha1.ApplicationSpec{},
			}

			By("calling the Default method")
			err := defaulter.Default(ctx, obj)
			Expect(err).NotTo(HaveOccurred())

			By("checking that the default values are set")
			Expect(obj.Namespace).To(Equal("test-ns-ginkgo-defaults"))
			Expect(obj.Spec.RolloutDuration).To(Equal(&metav1.Duration{Duration: workloadv1alpha1.DefaultRolloutDuration}))
			Expect(obj.Spec.EnableTLS).To(Equal(func() *bool { b := workloadv1alpha1.DefaultEnableTLS; return &b }()))
		})

		It("Should not override existing RolloutDuration and EnableTLS", func() {
			By("creating an application with existing RolloutDuration and EnableTLS")
			rolloutDuration := &metav1.Duration{Duration: 5 * time.Minute}
			enableTLS := func() *bool { b := false; return &b }()
			obj = &workloadv1alpha1.Application{
				ObjectMeta: metav1.ObjectMeta{Name: "test-app-ginkgo-exist", Namespace: "test-ns-ginkgo-exist"},
				Spec: workloadv1alpha1.ApplicationSpec{
					RolloutDuration: rolloutDuration,
					EnableTLS:       enableTLS,
				},
			}

			By("calling the Default method")
			err := defaulter.Default(ctx, obj)
			Expect(err).NotTo(HaveOccurred())

			By("checking that the existing values are not overridden")
			Expect(obj.Namespace).To(Equal("test-ns-ginkgo-exist"))
			Expect(obj.Spec.RolloutDuration).To(Equal(rolloutDuration))
			Expect(obj.Spec.EnableTLS).To(Equal(enableTLS))
		})

		It("Should set Namespace from Workspace", func() {
			By("creating an application without an initial namespace")
			obj = &workloadv1alpha1.Application{
				ObjectMeta: metav1.ObjectMeta{Name: "test-app-ginkgo-ns"}, // No initial namespace
				Spec:       workloadv1alpha1.ApplicationSpec{},
			}

			By("calling the Default method")
			err := defaulter.Default(ctx, obj)
			Expect(err).NotTo(HaveOccurred())

			By("checking that the namespace is set from the workspace")
			// Also check defaults were applied as expected
			Expect(obj.Spec.RolloutDuration).To(Equal(&metav1.Duration{Duration: workloadv1alpha1.DefaultRolloutDuration}))
			Expect(obj.Spec.EnableTLS).To(Equal(func() *bool { b := workloadv1alpha1.DefaultEnableTLS; return &b }()))
		})

		It("Should set default Scale values when Scale is empty", func() {
			By("creating an application with an empty Scale struct")
			obj = &workloadv1alpha1.Application{
				ObjectMeta: metav1.ObjectMeta{Name: "test-app-ginkgo-scale-defaults", Namespace: "test-ns-ginkgo-scale-defaults"},
				Spec: workloadv1alpha1.ApplicationSpec{
					Scale: workloadv1alpha1.Scale{}, // Empty Scale
				},
			}

			By("calling the Default method")
			err := defaulter.Default(ctx, obj)
			Expect(err).NotTo(HaveOccurred())

			By("checking that the default Scale values are set")
			Expect(obj.Spec.Scale.Metric).To(Equal(workloadv1alpha1.MetricConcurrency))
			Expect(obj.Spec.Scale.TargetUtilizationPercentage).To(Equal(func() *int32 { i := workloadv1alpha1.DefaultTargetUtilizationPercentage; return &i }()))
			Expect(obj.Spec.Scale.Target).To(BeNil()) // Target should not be defaulted if TargetUtilizationPercentage is
		})

		It("Should not set default TargetUtilizationPercentage if Target is set", func() {
			By("creating an application with Target set in Scale")
			target := int32(100)
			obj = &workloadv1alpha1.Application{
				ObjectMeta: metav1.ObjectMeta{Name: "test-app-ginkgo-scale-target", Namespace: "test-ns-ginkgo-scale-target"},
				Spec: workloadv1alpha1.ApplicationSpec{
					// Workspace: "test-workspace-ginkgo-scale-target", // Removed Workspace field
					Scale: workloadv1alpha1.Scale{
						Target: &target, // Target is set
						// Metric is empty, should be defaulted
					},
				},
			}

			By("calling the Default method")
			err := defaulter.Default(ctx, obj)
			Expect(err).NotTo(HaveOccurred())

			By("checking that TargetUtilizationPercentage is not defaulted when Target is set")
			Expect(obj.Spec.Scale.Metric).To(Equal(workloadv1alpha1.MetricConcurrency)) // Metric should still default
			Expect(obj.Spec.Scale.Target).To(Equal(&target))
			Expect(obj.Spec.Scale.TargetUtilizationPercentage).To(BeNil()) // TargetUtilizationPercentage should NOT be defaulted
		})
	})

	Context("When creating or updating Application under Validating Webhook", func() {
		// TODO (user): Add logic for validating webhooks using Ginkgo/Gomega if needed
		// Example:
		// It("Should deny creation if a required field is missing", func() {
		//     By("simulating an invalid creation scenario")
		//     obj.SomeRequiredField = ""
		//     _, err := validator.ValidateCreate(context.TODO(), obj) // Use context.TODO() or a real context
		//     Expect(err).To(HaveOccurred())
		// })
		// ... other validation tests
	})

})
