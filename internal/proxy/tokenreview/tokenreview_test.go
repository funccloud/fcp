package tokenreview

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	authv1 "k8s.io/api/authentication/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestTokenReview(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Token review Suite")
}

// testT struct to hold test case parameters
type testT struct {
	reviewResp *authv1.TokenReview
	errResp    error
	expAuth    bool
	expErr     error
}

var _ = Describe("TokenReview Review method", func() {
	var (
		tokenReviewer *TokenReview
		// mockClient will be client.Client, initialized by the fake client builder
		mockClient  client.Client
		httpRequest *http.Request
	)

	BeforeEach(func() {
		httpRequest = &http.Request{
			Header: map[string][]string{
				"Authorization": {"bearer test-token"},
			},
		}
	})

	Context("when handling token review requests", func() {
		testCases := map[string]testT{
			"if a create fails then this error is returned": {
				reviewResp: nil,
				errResp:    errors.New("create error response"),
				expAuth:    false,
				expErr:     errors.New("create error response"),
			},
			"if an error exists in the status of the response pass error back": {
				reviewResp: &authv1.TokenReview{
					Status: authv1.TokenReviewStatus{
						Error: "status error",
					},
				},
				errResp: nil,
				expAuth: false,
				expErr:  errors.New("error authenticating using token review: status error"),
			},
			"if the response returns unauthenticated, return false": {
				reviewResp: &authv1.TokenReview{
					Status: authv1.TokenReviewStatus{
						Authenticated: false,
					},
				},
				errResp: nil,
				expAuth: false,
				expErr:  nil,
			},
			"if the response returns authenticated, return true": {
				reviewResp: &authv1.TokenReview{
					Status: authv1.TokenReviewStatus{
						Authenticated: true,
					},
				},
				errResp: nil,
				expAuth: true,
				expErr:  nil,
			},
		}

		for desc, tc := range testCases {
			description := desc
			currentTest := tc // Capture range variable

			It(description, func() {
				builder := fake.NewClientBuilder().WithScheme(scheme.Scheme)

				interceptorFuncs := interceptor.Funcs{
					Create: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						tr, ok := obj.(*authv1.TokenReview)
						if !ok {
							// This test's mock is specific to TokenReview creation.
							return fmt.Errorf("fake client Create interceptor received unexpected type %T, expected *authv1.TokenReview", obj)
						}

						// Apply test-specific behavior from the captured currentTest
						if currentTest.errResp != nil {
							return currentTest.errResp
						}
						if currentTest.reviewResp != nil {
							tr.Status = currentTest.reviewResp.Status
						}
						return nil // Success, allow fake client to proceed with storing the (potentially modified) object
					},
				}
				builder.WithInterceptorFuncs(interceptorFuncs)
				mockClient = builder.Build()

				tokenReviewer = New(mockClient, nil)

				authed, err := tokenReviewer.Review(httpRequest)

				if currentTest.expErr != nil {
					Expect(err).To(HaveOccurred(), fmt.Sprintf("Expected an error but got none for test: %s", description))
					Expect(err.Error()).To(Equal(currentTest.expErr.Error()), fmt.Sprintf("Error message mismatch for test: %s", description))
				} else {
					Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Expected no error but got one: %v for test: %s", err, description))
				}
				Expect(authed).To(Equal(currentTest.expAuth), fmt.Sprintf("Authentication status mismatch for test: %s", description))
			})
		}
	})
})
