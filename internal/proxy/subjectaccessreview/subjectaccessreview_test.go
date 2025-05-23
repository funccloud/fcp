package subjectaccessreview

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	authzv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/authentication/user"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestSubjectAccessReview(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Subject access review Suite")
}

// stores the context for each test case
type testT struct {
	// the already authenticated user
	requester user.Info

	// the expected target information from the request
	expTarget user.Info

	// the expected authorization decision
	expAz bool

	// the expected error
	expErr error

	// expected error from rbacCheck
	expErrorRbac error

	// should the impersonation headers be found?
	expImpersonationHeaders bool

	// should include extra impersonation header?
	extraImpersonationHeader bool
}

var _ = Describe("SubjectAccessReview", func() {
	tests := map[string]testT{
		"if all reviews pass, user is authorized to impersonate": {
			requester: &user.DefaultInfo{
				Name:   "mmosley",
				Groups: []string{"group1", "group2"},
				Extra: map[string][]string{
					"remoteaddr": {"1.2.3.4"},
				},
			},

			expTarget: &user.DefaultInfo{
				Name:   "jjackson",
				Groups: []string{"group3"},
				Extra: map[string][]string{
					"remoteaddr": {"1.2.3.4"},
				},
				UID: "1-2-3-4",
			},

			expImpersonationHeaders:  true,
			expAz:                    true,
			expErr:                   nil,
			expErrorRbac:             nil,
			extraImpersonationHeader: false,
		},

		"user not authorized to impersonate target username": {
			requester: &user.DefaultInfo{
				Name:   "mmosley",
				Groups: []string{"group1", "group2"},
				Extra: map[string][]string{
					"remoteaddr": {"1.2.3.4"},
				},
			},

			expTarget: &user.DefaultInfo{
				Name:   "jjackson-x",
				Groups: []string{"group3"},
				Extra: map[string][]string{
					"remoteaddr": {"1.2.3.4"},
				},
				UID: "1-2-3-4",
			},

			expImpersonationHeaders:  true,
			expAz:                    false,
			expErr:                   errors.New("mmosley is not allowed to impersonate user 'jjackson-x'"),
			expErrorRbac:             nil,
			extraImpersonationHeader: false,
		},

		"user not authorized to impersonate target group": {
			requester: &user.DefaultInfo{
				Name:   "mmosley",
				Groups: []string{"group1", "group2"},
				Extra: map[string][]string{
					"remoteaddr": {"1.2.3.4"},
				},
				UID: "1-2-3-4",
			},

			expTarget: &user.DefaultInfo{
				Name:   "jjackson",
				Groups: []string{"group4"},
				Extra: map[string][]string{
					"remoteaddr": {"1.2.3.4"},
				},
				UID: "1-2-3-4",
			},

			expImpersonationHeaders:  true,
			expAz:                    false,
			expErr:                   errors.New("mmosley is not allowed to impersonate group 'group4'"),
			expErrorRbac:             nil,
			extraImpersonationHeader: false,
		},

		"user not authorized to impersonate target extraInfo": {
			requester: &user.DefaultInfo{
				Name:   "mmosley",
				Groups: []string{"group1", "group2"},
				Extra: map[string][]string{
					"remoteaddr": {"1.2.3.4"},
				},
				UID: "1-2-3-4",
			},

			expTarget: &user.DefaultInfo{
				Name:   "jjackson",
				Groups: []string{"group3"},
				Extra: map[string][]string{
					"remoteaddr": {"1.2.3.5"},
				},
				UID: "1-2-3-4",
			},

			expImpersonationHeaders:  true,
			expAz:                    false,
			expErr:                   errors.New("mmosley is not allowed to impersonate extra info 'remoteaddr'='1.2.3.5'"),
			expErrorRbac:             nil,
			extraImpersonationHeader: false,
		},

		"user is not authorized to impersonate the uid": {
			requester: &user.DefaultInfo{
				Name:   "mmosley",
				Groups: []string{"group1", "group2"},
				Extra: map[string][]string{
					"remoteaddr": {"1.2.3.4"},
				},
			},

			expTarget: &user.DefaultInfo{
				Name:   "jjackson",
				Groups: []string{"group3"},
				Extra: map[string][]string{
					"remoteaddr": {"1.2.3.4"},
				},
				UID: "1-2-3-5",
			},

			expImpersonationHeaders:  true,
			expAz:                    false,
			expErr:                   errors.New("mmosley is not allowed to impersonate uid '1-2-3-5'"),
			expErrorRbac:             nil,
			extraImpersonationHeader: false,
		},

		"error on the call returns false": {
			requester: &user.DefaultInfo{
				Name:   "mmosley-x",
				Groups: []string{"group1", "group2"},
				Extra: map[string][]string{
					"remoteaddr": {"1.2.3.4"},
				},
			},

			expTarget: &user.DefaultInfo{
				Name:   "jjackson",
				Groups: []string{"group3"},
				Extra: map[string][]string{
					"remoteaddr": {"1.2.3.4"},
				},
				UID: "1-2-3-4",
			},

			expImpersonationHeaders:  true,
			expAz:                    false,
			expErr:                   errors.New("error authorizing the request"),
			expErrorRbac:             errors.New("error authorizing the request"),
			extraImpersonationHeader: false,
		},

		"no impersonation headers found, should set flag as such": {
			requester: &user.DefaultInfo{
				Name:   "mmosley-x",
				Groups: []string{"group1", "group2"},
				Extra: map[string][]string{
					"remoteaddr": {"1.2.3.4"},
				},
			},

			expTarget: &user.DefaultInfo{},

			expImpersonationHeaders:  false,
			expAz:                    false,
			expErr:                   nil,
			expErrorRbac:             nil,
			extraImpersonationHeader: false,
		},

		"unknown impersonation header, error": {
			requester: &user.DefaultInfo{
				Name:   "mmosley-x",
				Groups: []string{"group1", "group2"},
				Extra: map[string][]string{
					"remoteaddr": {"1.2.3.4"},
				},
			},

			expTarget: &user.DefaultInfo{
				Name:   "jjackson",
				Groups: []string{"group3"},
				Extra: map[string][]string{
					"remoteaddr": {"1.2.3.4"},
				},
				UID: "1-2-3-4",
			},

			expImpersonationHeaders:  true,
			expAz:                    false,
			expErr:                   errors.New("unknown impersonation header 'Impersonate-doesnotexist'"),
			expErrorRbac:             nil,
			extraImpersonationHeader: true,
		},

		"missing impersonation-user": {
			requester: &user.DefaultInfo{
				Name:   "mmosley-x",
				Groups: []string{"group1", "group2"},
				Extra: map[string][]string{
					"remoteaddr": {"1.2.3.4"},
				},
			},

			expTarget: &user.DefaultInfo{
				Name:   "",
				Groups: []string{"group3"},
				Extra: map[string][]string{
					"remoteaddr": {"1.2.3.4"},
				},
				UID: "1-2-3-4",
			},

			expImpersonationHeaders:  true,
			expAz:                    false,
			expErr:                   errors.New("no Impersonation-User header found for request"),
			expErrorRbac:             nil,
			extraImpersonationHeader: false,
		},
	}

	for name, tc := range tests {
		Context(name, func() {
			It("should correctly authorize impersonation", func() {
				clientBuilder := fake.NewClientBuilder()
				scheme := runtime.NewScheme()
				Expect(authzv1.AddToScheme(scheme)).To(Succeed()) // Changed to Expect().To(Succeed())
				clientBuilder.WithScheme(scheme)

				// Capture tc for use in the interceptor
				currentTestCase := tc // Renamed for clarity

				interceptorFuncs := interceptor.Funcs{
					Create: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.CreateOption) error {

						sar, ok := obj.(*authzv1.SubjectAccessReview)
						if !ok {
							return fmt.Errorf("interceptor: unexpected object type: %T", obj)
						}

						if currentTestCase.expErrorRbac != nil {
							return currentTestCase.expErrorRbac
						}

						// Simulate SAR processing by the API server
						allowed := true // Default to allowed

						// This logic determines if the SAR should be denied based on the expected error message
						// matching the resource being checked in the SAR.
						if sar.Spec.ResourceAttributes != nil && currentTestCase.expErr != nil {
							ra := sar.Spec.ResourceAttributes
							errStr := currentTestCase.expErr.Error()
							deniedBasedOnExpectedError := false
							if ra.Resource == "users" && strings.Contains(errStr, fmt.Sprintf("not allowed to impersonate user '%s'", ra.Name)) {
								deniedBasedOnExpectedError = true
							} else if ra.Resource == "groups" && strings.Contains(errStr, fmt.Sprintf("not allowed to impersonate group '%s'", ra.Name)) {
								deniedBasedOnExpectedError = true
							} else if ra.Resource == "uids" && strings.Contains(errStr, fmt.Sprintf("not allowed to impersonate uid '%s'", ra.Name)) {
								deniedBasedOnExpectedError = true
							} else if ra.Resource == "userextras" {
								// The SUT uses ra.Subresource for the extra key and ra.Name for the extra value.
								expectedExtraDenialMsg := fmt.Sprintf("not allowed to impersonate extra info '%s'='%s'", ra.Subresource, ra.Name)
								if strings.Contains(errStr, expectedExtraDenialMsg) {
									deniedBasedOnExpectedError = true
								}
							}

							if deniedBasedOnExpectedError {
								allowed = false
							}
						}

						sar.Status.Allowed = allowed
						return nil // Indicate successful "creation" and processing by the interceptor
					},
				}
				clientBuilder.WithInterceptorFuncs(interceptorFuncs)

				fakeClient := clientBuilder.Build()

				testReviewer := New(fakeClient)

				headers := map[string][]string{}

				if currentTestCase.expImpersonationHeaders {
					if currentTestCase.expTarget.GetName() != "" {
						headers["Impersonate-User"] = []string{currentTestCase.expTarget.GetName()}
					}

					headers["Impersonate-Group"] = currentTestCase.expTarget.GetGroups()
					headers["Impersonate-Uid"] = []string{currentTestCase.expTarget.GetUID()}

					for key, value := range currentTestCase.expTarget.GetExtra() {
						headers["Impersonate-Extra-"+strings.ToUpper(key)] = value
					}

					if currentTestCase.extraImpersonationHeader {
						headers["Impersonate-doesnotexist"] = []string{"doesnotmatter"}
					}
				}

				target, err := testReviewer.CheckAuthorizedForImpersonation(
					&http.Request{
						Header: headers,
					}, currentTestCase.requester)

				// check if the errors match
				if currentTestCase.expErr != nil {
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(Equal(currentTestCase.expErr.Error()))
				} else {
					Expect(err).NotTo(HaveOccurred())
				}

				// check if impersonation was found when expected
				headersFound := !(err == nil && target == nil)
				Expect(headersFound).To(Equal(currentTestCase.expImpersonationHeaders))

				azSuccess := target != nil && err == nil
				// check if authorization matchs
				Expect(azSuccess).To(Equal(currentTestCase.expAz))

				// check that the final impersonated user lines up with the expected test case
				if azSuccess {
					Expect(target).To(Equal(currentTestCase.expTarget))
				} else {
					Expect(target).To(BeNil())
				}
			})
		})
	}
})
