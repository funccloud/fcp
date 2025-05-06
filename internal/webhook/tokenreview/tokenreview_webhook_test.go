package tokenreview

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	authv1 "k8s.io/api/authentication/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/authentication"
)

var (
	authenticator *Authenticator
)

var _ = Describe("TokenReview Webhook", func() {
	BeforeEach(func() {
		authenticator = &Authenticator{
			UserInfoEndpoint: s.URL,
		}
	})

	AfterEach(func() {
		// Add teardown logic here
	})

	Context("when handling a valid request", func() {
		It("should succeed", func() {
			resp := authenticator.Handle(ctx, authentication.Request{
				TokenReview: authv1.TokenReview{
					Spec: authv1.TokenReviewSpec{
						Token: "test-token",
					},
				},
			})
			Expect(resp.Status.Authenticated).To(BeTrue())
			Expect(resp.Status.User.Username).To(Equal("test-user"))
			Expect(resp.Status.User.UID).To(Equal("test-uid"))
			Expect(resp.Status.User.Groups).To(ContainElement("group1"))
			Expect(resp.Status.User.Groups).To(ContainElement("group2"))
			Expect(resp.Status.User.Extra["key"]).To(ContainElement("value1"))
			Expect(resp.Status.User.Extra["key"]).To(ContainElement("value2"))

		})
	})

	Context("when handling an invalid request", func() {
		It("should fail when token is empty", func() {
			resp := authenticator.Handle(ctx, authentication.Request{
				TokenReview: authv1.TokenReview{
					Spec: authv1.TokenReviewSpec{
						Token: "",
					},
				},
			})
			Expect(resp.Status.Authenticated).To(BeFalse())
			Expect(resp.Status.User.Username).To(BeEmpty())
			Expect(resp.Status.User.UID).To(BeEmpty())
			Expect(resp.Status.User.Groups).To(BeEmpty())
			Expect(resp.Status.User.Extra).To(BeEmpty())
			Expect(resp.Status.Error).To(Equal("missing token"))
		})

		It("should fail when token is invalid", func() {
			resp := authenticator.Handle(ctx, authentication.Request{
				TokenReview: authv1.TokenReview{
					Spec: authv1.TokenReviewSpec{
						Token: "",
					},
				},
			})
			Expect(resp.Status.Authenticated).To(BeFalse())
			Expect(resp.Status.User.Username).To(BeEmpty())
			Expect(resp.Status.User.UID).To(BeEmpty())
			Expect(resp.Status.User.Groups).To(BeEmpty())
			Expect(resp.Status.User.Extra).To(BeEmpty())
			Expect(resp.Status.Error).To(Equal("invalid token"))
		})
	})
})
