package mocks

import (
	"net/http"
	"time"

	"go.funccloud.dev/fcp/internal/proxy/tokenreview" // Ensured import
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/server"
	"k8s.io/apiserver/pkg/server/options"
	"k8s.io/client-go/rest"
)

// This package contains generated mocks
//go:generate ../../../bin/mockgen -package=mocks -destination authenticator.go k8s.io/apiserver/pkg/authentication/authenticator Token

// MockAuditOptions satisfies the proxy.AuditOptions interface (indirectly options.AuditOptions).
type MockAuditOptions struct {
	options.AuditOptions
}

func NewMockAuditOptions() *MockAuditOptions {
	return &MockAuditOptions{}
}

// MockRoundTripper is a mock for http.RoundTripper.
type MockRoundTripper struct {
	RoundTripFunc func(req *http.Request) (*http.Response, error)
}

func (m *MockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if m.RoundTripFunc != nil {
		return m.RoundTripFunc(req)
	}
	// Default behavior or error if not set
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: http.NoBody}, nil
}

func NewMockRoundTripper() *MockRoundTripper {
	return &MockRoundTripper{}
}

// MockTokenReviewer satisfies the tokenreview.TokenReviewerInterface.
var _ tokenreview.TokenReviewerInterface = &MockTokenReviewer{} // Compile-time check

type MockTokenReviewer struct {
	ReviewFunc func(req *http.Request) (passthrough bool, err error)
}

func NewMockTokenReviewer() *MockTokenReviewer {
	return &MockTokenReviewer{}
}

func (m *MockTokenReviewer) Review(req *http.Request) (passthrough bool, err error) {
	if m.ReviewFunc != nil {
		return m.ReviewFunc(req)
	}
	// Default mock behavior
	return false, nil
}

// MockSubjectAccessReviewer satisfies the subjectaccessreview.SubjectAccessReviewer interface.
type MockSubjectAccessReviewer struct{}

func NewMockSubjectAccessReviewer() *MockSubjectAccessReviewer {
	return &MockSubjectAccessReviewer{}
}

func (m *MockSubjectAccessReviewer) Review(ctx *http.Request, user user.Info, resourceAttributes runtime.Object) (bool, string, error) {
	// Basic mock implementation
	return true, "allowed", nil
}

// MockSecureServingInfo satisfies the proxy.SecureServingInfo interface (indirectly server.SecureServingInfo).
type MockSecureServingInfo struct {
	server.SecureServingInfo
}

func NewMockSecureServingInfo() *MockSecureServingInfo {
	return &MockSecureServingInfo{}
}

// Listener is a mock method
func (m *MockSecureServingInfo) Listener() (interface{}, error) {
	return nil, nil
}

// ApplyTo is a mock method
func (m *MockSecureServingInfo) ApplyTo(config *server.Config) error {
	return nil
}

// MockRestConfig satisfies the relevant parts of rest.Config used by the proxy.
type MockRestConfig struct {
	rest.Config
}

func NewMockRestConfig() *MockRestConfig {
	// Initialize with default values to avoid nil pointer dereferences if any fields are accessed.
	// This is a basic mock; specific fields might need to be set based on usage in proxy.New().
	return &MockRestConfig{
		Config: rest.Config{
			// Example: Set a Host if it's accessed, otherwise, keep it minimal.
			// Host: "mock-host",
			// Provide other defaults as necessary.
			Timeout: 30 * time.Second, // A common default
		},
	}
}
