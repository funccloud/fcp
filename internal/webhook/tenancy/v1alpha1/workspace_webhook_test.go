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
	"k8s.io/utils/ptr" // Import ptr package
	"sigs.k8s.io/controller-runtime/pkg/client"

	tenancyv1alpha1 "go.funccloud.dev/fcp/api/tenancy/v1alpha1"
	workloadv1alpha1 "go.funccloud.dev/fcp/api/workload/v1alpha1" // Import workload API
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
		validator.Client = k8sClient
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

		It("Should not change owners if already unique", func() {
			obj = &tenancyv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: workspaceName,
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       workspaceKind,
					APIVersion: tenancyv1alpha1.GroupVersion.String(),
				},
				Spec: tenancyv1alpha1.WorkspaceSpec{
					Type: tenancyv1alpha1.WorkspaceTypePersonal,
					Owners: []corev1.ObjectReference{{
						Kind: "User",
						Name: userName,
					}}, // Already unique
				},
			}
			err := defaulter.Default(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			By("checking that the owners list remains unchanged")
			Expect(obj.Spec.Owners).To(HaveLen(1))
			Expect(obj.Spec.Owners[0].Name).To(Equal(userName))
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
			// Setup old object for comparison
			oldObj = &tenancyv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					// Name: workspaceName, // Use userName to match the owner for personal type validity
					Name: userName,
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       workspaceKind,
					APIVersion: tenancyv1alpha1.GroupVersion.String(),
				},
				Spec: tenancyv1alpha1.WorkspaceSpec{
					Type: tenancyv1alpha1.WorkspaceTypePersonal,
					Owners: []corev1.ObjectReference{{
						Kind: "User",
						Name: userName,
					}},
				},
			}
			// Create a copy for the new object to modify
			obj = oldObj.DeepCopy()

			By("simulating an immutable field update (type)")
			// Temporarily set a different name for the type change test to avoid conflict with owner name rule
			obj.Name = workspaceName
			obj.Spec.Type = tenancyv1alpha1.WorkspaceTypeOrganization
			Expect(validator.ValidateUpdate(ctx, oldObj, obj)).Error().To(HaveOccurred())
			obj.Name = userName // Reset name

			By("simulating an immutable field update (owners for personal type)")
			obj.Spec.Type = tenancyv1alpha1.WorkspaceTypePersonal // Reset type
			obj.Spec.Owners = append(obj.Spec.Owners, corev1.ObjectReference{Kind: "User", Name: "another-user"})
			Expect(validator.ValidateUpdate(ctx, oldObj, obj)).Error().To(HaveOccurred())

			By("simulating a valid update (no immutable fields changed for personal)")
			obj.Spec.Owners = oldObj.Spec.Owners         // Reset owners
			obj.Labels = map[string]string{"foo": "bar"} // Change a mutable field
			// Now oldObj and obj represent a valid personal workspace state (name == owner name)
			// before the mutable label change, so the update should be allowed.
			Expect(validator.ValidateUpdate(ctx, oldObj, obj)).To(BeNil())

			By("simulating a valid update for organization type (changing owners)")
			// Change type in both old and new for this test scenario
			// Ensure oldObj name doesn't conflict with owner rule if it were personal
			oldObj.Name = workspaceName
			oldObj.Spec.Type = tenancyv1alpha1.WorkspaceTypeOrganization
			obj = oldObj.DeepCopy()
			obj.Spec.Owners = []corev1.ObjectReference{{
				Kind: "Group",
				Name: "new-test-group",
			}}
			Expect(validator.ValidateUpdate(ctx, oldObj, obj)).To(BeNil())
		})

		It("Should validate creation correctly", func() {
			By("creating a personal workspace with more than one owner")
			obj = &tenancyv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: workspaceName,
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       workspaceKind,
					APIVersion: tenancyv1alpha1.GroupVersion.String(),
				},
				Spec: tenancyv1alpha1.WorkspaceSpec{
					Type: tenancyv1alpha1.WorkspaceTypePersonal,
					Owners: []corev1.ObjectReference{{
						Kind: "User",
						Name: userName,
					}, {
						Kind: "User",
						Name: "another-user",
					}}, // More than one owner
				},
			}
			Expect(validator.ValidateCreate(ctx, obj)).Error().To(HaveOccurred())

			By("creating a organization workspace with multiple owners")
			obj.Spec.Type = tenancyv1alpha1.WorkspaceTypeOrganization
			obj.Spec.Owners = []corev1.ObjectReference{{
				Kind: "User",
				Name: userName,
			}, {
				Kind: "Group",
				Name: "test-group",
			}}
			Expect(validator.ValidateCreate(ctx, obj)).To(BeNil())

			By("creating a personal workspace with one owner")
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
			// Setup old object for comparison
			oldObj = &tenancyv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: workspaceName,
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       workspaceKind,
					APIVersion: tenancyv1alpha1.GroupVersion.String(),
				},
				Spec: tenancyv1alpha1.WorkspaceSpec{
					Type: tenancyv1alpha1.WorkspaceTypePersonal,
					Owners: []corev1.ObjectReference{{
						Kind: "User",
						Name: userName,
					}},
				},
			}
			// Create a copy for the new object to modify
			obj = oldObj.DeepCopy()

			By("simulating an immutable field update (type)")
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

		It("Should validate updates correctly", func() {
			// Setup old object for comparison
			oldObj = &tenancyv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					// Name: workspaceName, // Use userName to match the owner for personal type validity
					Name: userName,
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       workspaceKind,
					APIVersion: tenancyv1alpha1.GroupVersion.String(),
				},
				Spec: tenancyv1alpha1.WorkspaceSpec{
					Type: tenancyv1alpha1.WorkspaceTypePersonal,
					Owners: []corev1.ObjectReference{{
						Kind: "User",
						Name: userName,
					}},
				},
			}
			// Create a copy for the new object to modify
			obj = oldObj.DeepCopy()

			By("simulating an immutable field update (type)")
			// Temporarily set a different name for the type change test to avoid conflict with owner name rule
			obj.Name = workspaceName
			obj.Spec.Type = tenancyv1alpha1.WorkspaceTypeOrganization
			Expect(validator.ValidateUpdate(ctx, oldObj, obj)).Error().To(HaveOccurred())
			obj.Name = userName // Reset name

			By("simulating an immutable field update (owners for personal type)")
			obj.Spec.Type = tenancyv1alpha1.WorkspaceTypePersonal // Reset type
			obj.Spec.Owners = append(obj.Spec.Owners, corev1.ObjectReference{Kind: "User", Name: "another-user"})
			Expect(validator.ValidateUpdate(ctx, oldObj, obj)).Error().To(HaveOccurred())

			By("simulating a valid update (no immutable fields changed for personal)")
			obj.Spec.Owners = oldObj.Spec.Owners         // Reset owners
			obj.Labels = map[string]string{"foo": "bar"} // Change a mutable field
			// Now oldObj and obj represent a valid personal workspace state (name == owner name)
			// before the mutable label change, so the update should be allowed.
			Expect(validator.ValidateUpdate(ctx, oldObj, obj)).To(BeNil())

			By("simulating a valid update for organization type (changing owners)")
			// Change type in both old and new for this test scenario
			// Ensure oldObj name doesn't conflict with owner rule if it were personal
			oldObj.Name = workspaceName
			oldObj.Spec.Type = tenancyv1alpha1.WorkspaceTypeOrganization
			obj = oldObj.DeepCopy()
			obj.Spec.Owners = []corev1.ObjectReference{{
				Kind: "Group",
				Name: "new-test-group",
			}}
			Expect(validator.ValidateUpdate(ctx, oldObj, obj)).To(BeNil())
		})
	})

	Context("When deleting Workspace under Validating Webhook", func() {
		var testNamespace *corev1.Namespace
		BeforeEach(func() {
			// Create the test namespace with a generated name to avoid conflicts
			testNamespace = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{GenerateName: workspaceName + "-"},
			}
			Expect(k8sClient.Create(ctx, testNamespace)).Should(Succeed())
			// Refresh testNamespace to get the actual generated name
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(testNamespace), testNamespace)).Should(Succeed())

			// Initialize the validator with the test environment client
			validator = WorkspaceCustomValidator{Client: k8sClient}
			Expect(validator.Client).NotTo(BeNil(), "Validator client should be initialized")

			// Basic workspace object for deletion tests - use the constant name for the workspace itself
			obj = &tenancyv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: workspaceName,
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       workspaceKind,
					APIVersion: tenancyv1alpha1.GroupVersion.String(),
				},
				Spec: tenancyv1alpha1.WorkspaceSpec{
					Type: tenancyv1alpha1.WorkspaceTypeOrganization, // Type doesn't matter much for delete validation
					Owners: []corev1.ObjectReference{{
						Kind: "Group",
						Name: "test-group",
					}},
				},
			}
		})

		AfterEach(func() {
			// Clean up any created applications within the test namespace
			appList := &workloadv1alpha1.ApplicationList{}
			err := k8sClient.List(ctx, appList, client.InNamespace(testNamespace.Name))
			if err == nil {
				for _, app := range appList.Items {
					Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &app))).Should(Succeed())
				}
			}

			// Delete the test namespace and wait for it to be gone
			if testNamespace != nil && testNamespace.Name != "" {
				Expect(k8sClient.Delete(ctx, testNamespace)).Should(Succeed())
				Eventually(func() error {
					ns := &corev1.Namespace{}
					return k8sClient.Get(ctx, client.ObjectKey{Name: testNamespace.Name}, ns)
				}, "10s", "100ms").Should(HaveOccurred()) // Expect Get to fail (NotFound)
			}
		})

		It("Should deny deletion if associated Applications exist", func() {
			By("creating an associated Application")
			app := &workloadv1alpha1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: testNamespace.Name,
					Labels: map[string]string{
						tenancyv1alpha1.WorkspaceLinkedResourceLabel: workspaceName,
					},
				},
				Spec: workloadv1alpha1.ApplicationSpec{
					Workspace: workspaceName,
					Image:     "test-image",
					Scale: workloadv1alpha1.Scale{
						MinReplicas: ptr.To[int32](0),
						MaxReplicas: ptr.To[int32](1),
					},
				},
			}
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())

			By("validating workspace deletion")
			_, err := validator.ValidateDelete(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("workspace is not empty"))
		})

		It("Should allow deletion if no associated Applications exist", func() {
			By("ensuring no associated Applications exist")
			// No app created in this test case

			By("validating workspace deletion")
			_, err := validator.ValidateDelete(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})
	})

}) // End Describe
