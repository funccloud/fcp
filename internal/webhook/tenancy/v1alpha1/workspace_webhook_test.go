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
	"fmt" // Add fmt import

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tenancyv1alpha1 "go.funccloud.dev/fcp/api/tenancy/v1alpha1"
	workloadv1alpha1 "go.funccloud.dev/fcp/api/workload/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	})

	AfterEach(func() {
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
					}},
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
			obj = oldObj.DeepCopy()
			By("simulating an immutable field update (type)")
			obj.Name = workspaceName
			obj.Spec.Type = tenancyv1alpha1.WorkspaceTypeOrganization
			Expect(validator.ValidateUpdate(ctx, oldObj, obj)).Error().To(HaveOccurred())
			obj.Name = userName

			By("simulating an immutable field update (owners for personal type)")
			obj.Spec.Type = tenancyv1alpha1.WorkspaceTypePersonal // Reset type
			obj.Spec.Owners = append(obj.Spec.Owners, corev1.ObjectReference{Kind: "User", Name: "another-user"})
			Expect(validator.ValidateUpdate(ctx, oldObj, obj)).Error().To(HaveOccurred())

			By("simulating a valid update (no immutable fields changed for personal)")
			obj.Spec.Owners = oldObj.Spec.Owners
			obj.Labels = map[string]string{"foo": "bar"}
			Expect(validator.ValidateUpdate(ctx, oldObj, obj)).To(BeNil())

			By("simulating a valid update for organization type (changing owners)")
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
			oldObj = &tenancyv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
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
			obj = oldObj.DeepCopy()

			By("simulating an immutable field update (type)")
			obj.Name = workspaceName
			obj.Spec.Type = tenancyv1alpha1.WorkspaceTypeOrganization
			Expect(validator.ValidateUpdate(ctx, oldObj, obj)).Error().To(HaveOccurred())
			obj.Name = userName

			By("simulating an immutable field update (owners for personal type)")
			obj.Spec.Type = tenancyv1alpha1.WorkspaceTypePersonal // Reset type
			obj.Spec.Owners = append(obj.Spec.Owners, corev1.ObjectReference{Kind: "User", Name: "another-user"})
			Expect(validator.ValidateUpdate(ctx, oldObj, obj)).Error().To(HaveOccurred())

			By("simulating a valid update (no immutable fields changed for personal)")
			obj.Spec.Owners = oldObj.Spec.Owners
			obj.Labels = map[string]string{"foo": "bar"}
			Expect(validator.ValidateUpdate(ctx, oldObj, obj)).To(BeNil())

			By("simulating a valid update for organization type (changing owners)")
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
			testNamespace = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{GenerateName: workspaceName + "-"},
			}
			Expect(k8sClient.Create(ctx, testNamespace)).Should(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(testNamespace), testNamespace)).Should(Succeed())

			ws := &tenancyv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: testNamespace.Name,
				},
				Spec: tenancyv1alpha1.WorkspaceSpec{
					Type: tenancyv1alpha1.WorkspaceTypeOrganization,
					Owners: []corev1.ObjectReference{{
						Kind: "User",
						Name: "test-deleter",
					}},
				},
			}
			Expect(k8sClient.Create(ctx, ws)).Should(Succeed())

			validator = WorkspaceCustomValidator{Client: k8sClient}
			Expect(validator.Client).NotTo(BeNil(), "Validator client should be initialized")
			obj = &tenancyv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: workspaceName,
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       workspaceKind,
					APIVersion: tenancyv1alpha1.GroupVersion.String(),
				},
				Spec: tenancyv1alpha1.WorkspaceSpec{
					Type: tenancyv1alpha1.WorkspaceTypeOrganization,
					Owners: []corev1.ObjectReference{{
						Kind: "Group",
						Name: "test-group",
					}},
				},
			}
		})

		AfterEach(func() {
			appList := &workloadv1alpha1.ApplicationList{}
			listOpts := []client.ListOption{
				client.InNamespace(testNamespace.Name),
			}
			err := k8sClient.List(ctx, appList, listOpts...)
			if err == nil {
				for i := range appList.Items {
					app := appList.Items[i]
					By(fmt.Sprintf("Deleting application %s/%s", app.Namespace, app.Name))
					deleteErr := k8sClient.Delete(ctx, &app)
					if deleteErr != nil && !apierrors.IsNotFound(deleteErr) {
						Fail(fmt.Sprintf("Failed to start deletion of application %s/%s: %v", app.Namespace, app.Name, deleteErr))
					}

					By(fmt.Sprintf("Waiting for application %s/%s to be deleted", app.Namespace, app.Name))
					Eventually(func() error {
						tempApp := &workloadv1alpha1.Application{}
						getErr := k8sClient.Get(ctx, client.ObjectKey{Namespace: app.Namespace, Name: app.Name}, tempApp)
						return getErr
					}, "30s", "250ms").Should(Satisfy(apierrors.IsNotFound), fmt.Sprintf("Application %s/%s should be deleted", app.Namespace, app.Name))
				}
			} else if !apierrors.IsNotFound(err) {
				_, ierr := fmt.Fprintf(GinkgoWriter, "Warning: Failed to list applications in namespace %s during cleanup: %v\\n", testNamespace.Name, err)
				Expect(ierr).NotTo(HaveOccurred(), "Failed to list applications in test namespace")
			}

			if testNamespace != nil && testNamespace.Name != "" {
				By(fmt.Sprintf("Deleting test namespace %s", testNamespace.Name))
				deleteErr := k8sClient.Delete(ctx, testNamespace)
				if deleteErr != nil && !apierrors.IsNotFound(deleteErr) {
					Fail(fmt.Sprintf("Failed to start deletion of namespace %s: %v", testNamespace.Name, deleteErr))
				}
			}
		})

		It("Should deny deletion if associated Applications exist", func() {
			By("creating an associated Application")
			app := &workloadv1alpha1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: testNamespace.Name,
					// Add the label that ValidateDelete checks for
					Labels: map[string]string{
						tenancyv1alpha1.WorkspaceLinkedResourceLabel: testNamespace.Name,
					},
				},
				Spec: workloadv1alpha1.ApplicationSpec{
					Containers: []corev1.Container{
						{
							Name:  "test-container",
							Image: "test-image",
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 80,
								},
							},
						},
					},
					Scale: workloadv1alpha1.Scale{
						MinReplicas: ptr.To[int32](0),
						MaxReplicas: ptr.To[int32](1),
					},
				},
			}
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())

			// Ensure the application is findable by label before validating deletion
			Eventually(func() error {
				appList := &workloadv1alpha1.ApplicationList{}
				return k8sClient.List(ctx, appList, client.MatchingLabels{
					tenancyv1alpha1.WorkspaceLinkedResourceLabel: testNamespace.Name,
				}, client.Limit(1))
			}, "5s", "250ms").Should(Succeed(), "Application should be listable by label")

			By("validating workspace deletion")
			// Use the workspace object created in BeforeEach, ensuring its name matches the namespace
			obj.Name = testNamespace.Name
			_, err := validator.ValidateDelete(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("workspace cannot be deleted because it contains 1 application(s)"))
		})

		It("Should allow deletion if no associated Applications exist", func() {
			By("ensuring no associated Applications exist")
			// (No application created in this test's scope)
			By("validating workspace deletion")
			// Use the workspace object created in BeforeEach, ensuring its name matches the namespace
			obj.Name = testNamespace.Name
			_, err := validator.ValidateDelete(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})
	})

})
