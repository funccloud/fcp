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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	tenancyv1alpha1 "go.funccloud.dev/fcp/api/tenancy/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const workspaceKind = "Workspace"
const workspaceName = "test-workspace"
const userName = "test-user"

var _ = Describe("Workspace Webhook", func() {
	var (
		obj       *tenancyv1alpha1.Workspace
		oldObj    *tenancyv1alpha1.Workspace
		validator WorkspaceCustomValidator
		defaulter WorkspaceCustomDefaulter
	)

	BeforeEach(func() {
		obj = &tenancyv1alpha1.Workspace{}
		oldObj = &tenancyv1alpha1.Workspace{}
		validator = WorkspaceCustomValidator{}
		Expect(validator).NotTo(BeNil(), "Expected validator to be initialized")
		defaulter = WorkspaceCustomDefaulter{}
		Expect(defaulter).NotTo(BeNil(), "Expected defaulter to be initialized")
		Expect(oldObj).NotTo(BeNil(), "Expected oldObj to be initialized")
		Expect(obj).NotTo(BeNil(), "Expected obj to be initialized")
		// TODO (user): Add any setup logic common to all tests
	})

	AfterEach(func() {
		// TODO (user): Add any teardown logic common to all tests
	})

	Context("When creating Workspace under Defaulting Webhook", func() {
		It("Should apply defaults", func() {
			obj = &tenancyv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: workspaceName,
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "Workspace",
					APIVersion: tenancyv1alpha1.GroupVersion.String(),
				},
				Spec: tenancyv1alpha1.WorkspaceSpec{
					Type: tenancyv1alpha1.WorkspaceTypePersonal,
					Owners: []corev1.ObjectReference{{
						Kind: "User",
						Name: userName,
					}, {
						Kind: "User",
						Name: userName,
					}},
				},
			}
			err := defaulter.Default(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			By("checking that the owners are unique")
			Expect(obj.Spec.Owners).To(HaveLen(1))
		})
	})

	Context("When creating or updating Workspace under Validating Webhook", func() {
		obj = &tenancyv1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name: workspaceName,
			},
			TypeMeta: metav1.TypeMeta{
				Kind:       "Workspace",
				APIVersion: tenancyv1alpha1.GroupVersion.String(),
			},
			Spec: tenancyv1alpha1.WorkspaceSpec{
				Type: tenancyv1alpha1.WorkspaceTypePersonal,
				Owners: []corev1.ObjectReference{{
					Kind: "User",
					Name: userName,
				}, {
					Kind: "User",
					Name: userName,
				}},
			},
		}
		oldObj = &tenancyv1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name: workspaceName,
			},
			TypeMeta: metav1.TypeMeta{
				Kind:       "Workspace",
				APIVersion: tenancyv1alpha1.GroupVersion.String(),
			},
			Spec: tenancyv1alpha1.WorkspaceSpec{
				Type: tenancyv1alpha1.WorkspaceTypePersonal,
				Owners: []corev1.ObjectReference{{
					Kind: "User",
					Name: userName,
				}, {
					Kind: "User",
					Name: userName,
				}},
			},
		}
		It("Should deny creation if a required field is missing", func() {
			obj.Kind = workspaceKind
			obj.APIVersion = tenancyv1alpha1.GroupVersion.String()
			obj.Kind = workspaceKind
			obj.Name = workspaceName
			Expect(validator.ValidateCreate(ctx, obj)).Error().To(HaveOccurred())
			By("simulating an invalid workspace type")
			obj.Spec.Type = "invalid_type"
			obj.APIVersion = tenancyv1alpha1.GroupVersion.String()
			obj.Kind = workspaceKind
			obj.Name = workspaceName
			Expect(validator.ValidateCreate(ctx, obj)).Error().To(HaveOccurred())
		})
		//
		It("Should admit creation if all required fields are present", func() {
			By("simulating a valid creation scenario")
			obj.Spec.Type = tenancyv1alpha1.WorkspaceTypePersonal
			obj.APIVersion = tenancyv1alpha1.GroupVersion.String()
			obj.Kind = workspaceKind
			obj.Name = userName
			obj.Spec.Owners = []corev1.ObjectReference{{
				Kind: "User",
				Name: userName,
			}}
			Expect(validator.ValidateCreate(ctx, obj)).To(BeNil())
		})

		It("Should validate updates correctly", func() {
			By("simulating a imutable field update")
			obj.Spec.Type = tenancyv1alpha1.WorkspaceTypeOrganization
			Expect(validator.ValidateUpdate(ctx, oldObj, obj)).Error().To(HaveOccurred())
			By("simulating a update to a valid field")
			obj.Spec.Type = tenancyv1alpha1.WorkspaceTypePersonal
			obj.APIVersion = tenancyv1alpha1.GroupVersion.String()
			obj.Kind = workspaceKind
			obj.Name = "test-user"
			obj.Spec.Owners = []corev1.ObjectReference{{
				Kind: "User",
				Name: userName,
			}}
			oldObj.Spec.Type = tenancyv1alpha1.WorkspaceTypePersonal
			oldObj.APIVersion = tenancyv1alpha1.GroupVersion.String()
			oldObj.Kind = workspaceKind
			oldObj.Name = userName
			oldObj.Spec.Owners = []corev1.ObjectReference{{
				Kind: "User",
				Name: userName,
			}}
			Expect(validator.ValidateUpdate(ctx, oldObj, obj)).To(BeNil())
		})
	})

})
