package logging

import (
	"net/http"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAccessLog(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AccessLog Suite")
}

var _ = Describe("XForwardedFor", func() {
	tests := map[string]struct {
		headers    http.Header
		remoteAddr string
		exp        string
	}{
		"no x-forwarded-for": {
			headers:    http.Header{},
			remoteAddr: "1.2.3.4",
			exp:        "",
		},
		"empty x-forwarded-for": {
			headers: http.Header{
				"X-Forwarded-For": []string{""},
			},
			remoteAddr: "1.2.3.4",
			exp:        "",
		},
		"x-forwarded-for is remoteaddr": {
			headers: http.Header{
				"X-Forwarded-For": []string{"1.2.3.4"},
			},
			remoteAddr: "1.2.3.4",
			exp:        "",
		},
		"x-forwarded-for with no remoteaddr": {
			headers: http.Header{
				"X-Forwarded-For": []string{"1.2.3.1"},
			},
			remoteAddr: "1.2.3.4",
			exp:        "1.2.3.1",
		},
		"x-forwarded-for with with remoteaddr at the end": {
			headers: http.Header{
				"X-Forwarded-For": []string{"1.2.3.1, 1.2.3.4"},
			},
			remoteAddr: "1.2.3.4",
			exp:        "1.2.3.1",
		},
		"x-forwarded-for with with remoteaddr at the beginning": {
			headers: http.Header{
				"X-Forwarded-For": []string{"1.2.3.4, 1.2.3.1"},
			},
			remoteAddr: "1.2.3.4",
			exp:        "1.2.3.1",
		},
	}

	for name, test := range tests {
		It(name, func() {
			forwarded := findXForwardedFor(test.headers, test.remoteAddr)
			Expect(forwarded).To(Equal(test.exp))
		})
	}
})
