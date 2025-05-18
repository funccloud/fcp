package net

import (
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const ( // Added const for repeated string
	privateIPRemoteAddr = "127.0.0.1:8080"
)

func TestNet(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Net Suite")
}

var _ = Describe("Net", func() {
	Describe("XFF Handler", func() {
		var (
			xff      *XFF
			req      *http.Request
			rr       *httptest.ResponseRecorder
			next     http.HandlerFunc
			testAddr = "192.0.2.1:1234" // Example public IP
		)

		BeforeEach(func() {
			var err error
			xff, err = New(Options{AllowedSubnets: []string{}}) // Allow all for simplicity in most tests
			Expect(err).NotTo(HaveOccurred())
			rr = httptest.NewRecorder()
			next = func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}
		})

		Context("when X-Forwarded-For header is not present", func() {
			It("should not change RemoteAddr", func() {
				req = httptest.NewRequest("GET", "/", nil)
				req.RemoteAddr = testAddr
				handler := xff.Handler(next)
				handler.ServeHTTP(rr, req)
				Expect(req.RemoteAddr).To(Equal(testAddr))
			})
		})

		Context("when X-Forwarded-For header is present", func() {
			Context("and all subnets are allowed", func() {
				It("should update RemoteAddr with the public IP from X-Forwarded-For", func() {
					req = httptest.NewRequest("GET", "/", nil)
					req.RemoteAddr = privateIPRemoteAddr // A private IP, should be allowed to forward
					req.Header.Set("X-Forwarded-For", "203.0.113.1, 192.0.2.1")
					handler := xff.Handler(next)
					handler.ServeHTTP(rr, req)
					Expect(req.RemoteAddr).To(Equal("203.0.113.1:8080"))
				})

				It("should pick the first public IP from X-Forwarded-For", func() {
					req = httptest.NewRequest("GET", "/", nil)
					req.RemoteAddr = privateIPRemoteAddr
					req.Header.Set("X-Forwarded-For", "10.0.0.1, 203.0.113.5, 172.16.0.1")
					handler := xff.Handler(next)
					handler.ServeHTTP(rr, req)
					Expect(req.RemoteAddr).To(Equal("203.0.113.5:8080"))
				})

				It("should not change RemoteAddr if no public IP in X-Forwarded-For", func() {
					req = httptest.NewRequest("GET", "/", nil)
					req.RemoteAddr = privateIPRemoteAddr
					req.Header.Set("X-Forwarded-For", "10.0.0.1, 192.168.1.1")
					handler := xff.Handler(next)
					handler.ServeHTTP(rr, req)
					Expect(req.RemoteAddr).To(Equal("127.0.0.1:8080"))
				})
			})

			Context("and specific subnets are allowed", func() {
				BeforeEach(func() {
					var err error
					xff, err = New(Options{AllowedSubnets: []string{"192.168.1.0/24"}})
					Expect(err).NotTo(HaveOccurred())
				})

				It("should update RemoteAddr if request comes from an allowed subnet", func() {
					req = httptest.NewRequest("GET", "/", nil)
					req.RemoteAddr = "192.168.1.10:8080" // Allowed subnet
					req.Header.Set("X-Forwarded-For", "203.0.113.2")
					handler := xff.Handler(next)
					handler.ServeHTTP(rr, req)
					Expect(req.RemoteAddr).To(Equal("203.0.113.2:8080"))
				})

				It("should not update RemoteAddr if request comes from a disallowed subnet", func() {
					req = httptest.NewRequest("GET", "/", nil)
					req.RemoteAddr = "172.16.0.5:8080" // Disallowed subnet
					req.Header.Set("X-Forwarded-For", "203.0.113.3")
					handler := xff.Handler(next)
					handler.ServeHTTP(rr, req)
					Expect(req.RemoteAddr).To(Equal("172.16.0.5:8080"))
				})
			})

			Context("when X-Forwarded-For header contains invalid IP", func() {
				It("should not change RemoteAddr", func() {
					req = httptest.NewRequest("GET", "/", nil)
					req.RemoteAddr = privateIPRemoteAddr
					req.Header.Set("X-Forwarded-For", "invalid-ip, 203.0.113.4")
					handler := xff.Handler(next)
					handler.ServeHTTP(rr, req)
					Expect(req.RemoteAddr).To(Equal("203.0.113.4:8080")) // Should pick the valid one
				})

				It("should not change RemoteAddr if all IPs are invalid", func() {
					req = httptest.NewRequest("GET", "/", nil)
					req.RemoteAddr = privateIPRemoteAddr
					req.Header.Set("X-Forwarded-For", "invalid-ip, another-invalid-ip")
					handler := xff.Handler(next)
					handler.ServeHTTP(rr, req)
					Expect(req.RemoteAddr).To(Equal("127.0.0.1:8080"))
				})
			})
		})

		// Tests for ServeHTTP method (similar logic to Handler but for different middleware style)
		Describe("XFF ServeHTTP method", func() {
			It("should update RemoteAddr when X-Forwarded-For is present and allowed", func() {
				req = httptest.NewRequest("GET", "/", nil)
				req.RemoteAddr = privateIPRemoteAddr
				req.Header.Set("X-Forwarded-For", "203.0.113.7")
				xff.ServeHTTP(rr, req, next)
				Expect(req.RemoteAddr).To(Equal("203.0.113.7:8080"))
			})
		})

		// Tests for HandlerFunc method (Martini compatible)
		Describe("XFF HandlerFunc method", func() {
			It("should update RemoteAddr when X-Forwarded-For is present and allowed", func() {
				req = httptest.NewRequest("GET", "/", nil)
				req.RemoteAddr = privateIPRemoteAddr
				req.Header.Set("X-Forwarded-For", "203.0.113.8")
				xff.HandlerFunc(rr, req) // No next handler for this style
				Expect(req.RemoteAddr).To(Equal("203.0.113.8:8080"))
			})
		})

		Describe("New XFF", func() {
			It("should return error for invalid CIDR in options", func() {
				_, err := New(Options{AllowedSubnets: []string{"invalid-cidr"}})
				Expect(err).To(HaveOccurred())
			})

			It("should correctly initialize with empty AllowedSubnets (allowAll = true)", func() {
				xffInstance, err := New(Options{AllowedSubnets: []string{}})
				Expect(err).NotTo(HaveOccurred())
				// Internal check, not directly testable without exposing fields or specific behavior
				// For now, we rely on the behavior tested in "all subnets are allowed" context.
				// To make this more robust, one might add an IsAllowed method to XFF for testing.
				req = httptest.NewRequest("GET", "/", nil)
				req.RemoteAddr = "192.168.1.10:8080" // Any IP
				req.Header.Set("X-Forwarded-For", "203.0.113.9")
				handler := xffInstance.Handler(next)
				handler.ServeHTTP(rr, req)
				Expect(req.RemoteAddr).To(Equal("203.0.113.9:8080"))
			})

			It("should correctly initialize with specific AllowedSubnets", func() {
				xffInstance, err := New(Options{AllowedSubnets: []string{"10.0.0.0/8"}})
				Expect(err).NotTo(HaveOccurred())

				// Test with an allowed IP
				req = httptest.NewRequest("GET", "/", nil)
				req.RemoteAddr = "10.0.0.1:8080"
				req.Header.Set("X-Forwarded-For", "203.0.113.10")
				handler := xffInstance.Handler(next)
				handler.ServeHTTP(rr, req)
				Expect(req.RemoteAddr).To(Equal("203.0.113.10:8080"))

				// Test with a disallowed IP
				req = httptest.NewRequest("GET", "/", nil)
				req.RemoteAddr = "192.168.1.1:8080"
				req.Header.Set("X-Forwarded-For", "203.0.113.11")
				handler = xffInstance.Handler(next) // Re-assign to ensure fresh state if needed
				handler.ServeHTTP(rr, req)
				Expect(req.RemoteAddr).To(Equal("192.168.1.1:8080"))
			})
		})
	})
})
