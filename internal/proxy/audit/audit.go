package audit

import (
	"fmt"
	"net/http"

	"k8s.io/apimachinery/pkg/util/sets"
	genericapifilters "k8s.io/apiserver/pkg/endpoints/filters"
	"k8s.io/apiserver/pkg/server"
	genericfilters "k8s.io/apiserver/pkg/server/filters"
	"k8s.io/apiserver/pkg/server/options"
	"k8s.io/component-base/compatibility"
)

type Audit struct {
	serverConfig *server.CompletedConfig
	Opts         *options.AuditOptions
}

// New creates a new Audit struct to handle auditing for proxy requests. This
// is mostly a wrapper for the apiserver auditing handlers to combine them with
// the proxy.
func New(opts *options.AuditOptions, externalAddress string, secureServingInfo *server.SecureServingInfo) (*Audit, error) {
	serverConfig := &server.Config{
		ExternalAddress: externalAddress,
		SecureServing:   secureServingInfo,

		// Default to treating watch as a long-running operation.
		// Generic API servers have no inherent long-running subresources.
		// This is so watch requests are handled correctly in the audit log.
		LongRunningFunc: genericfilters.BasicLongRunningRequestCheck(
			sets.NewString("watch"), sets.NewString()),
	}

	// We do not support dynamic auditing, so leave nil
	if err := opts.ApplyTo(serverConfig); err != nil {
		return nil, err
	}
	serverConfig.EffectiveVersion = compatibility.NewEffectiveVersionFromString("1.0.31", "", "")
	completed := serverConfig.Complete(nil)

	return &Audit{
		Opts:         opts,
		serverConfig: &completed,
	}, nil
}

// Run will run the audit backend if configured.
func (a *Audit) Run(stopCh <-chan struct{}) error {
	if a.serverConfig.AuditBackend != nil {
		if err := a.serverConfig.AuditBackend.Run(stopCh); err != nil {
			return fmt.Errorf("failed to run the audit backend: %s", err)
		}
	}

	return nil
}

// Shutdown will shutdown the audit backend if configured.
func (a *Audit) Shutdown() error {
	if a.serverConfig.AuditBackend != nil {
		a.serverConfig.AuditBackend.Shutdown()
	}

	return nil
}

// WithRequest will wrap the given handler to inject the request information
// into the context which is then used by the wrapped audit handler.
func (a *Audit) WithRequest(handler http.Handler) http.Handler {
	handler = genericapifilters.WithAudit(handler, a.serverConfig.AuditBackend, a.serverConfig.AuditPolicyRuleEvaluator, a.serverConfig.LongRunningFunc)
	handler = genericapifilters.WithAuditInit(handler)
	return genericapifilters.WithRequestInfo(handler, a.serverConfig.RequestInfoResolver)
}

// WithUnauthorized will wrap the given handler to inject the request
// information into the context which is then used by the wrapped audit
// handler.
func (a *Audit) WithUnauthorized(handler http.Handler) http.Handler {
	handler = genericapifilters.WithFailedAuthenticationAudit(handler, a.serverConfig.AuditBackend, a.serverConfig.AuditPolicyRuleEvaluator)
	return genericapifilters.WithRequestInfo(handler, a.serverConfig.RequestInfoResolver)
}

// This struct is used to implement an http.Handler interface. This will not
// actually serve but instead implements auditing during unauthenticated
// requests. It is expected that consumers of this type will call `ServeHTTP`
// when an unauthenticated request is received.
type unauthenticatedHandler struct {
	serveFunc func(http.ResponseWriter, *http.Request)
}

func NewUnauthenticatedHandler(a *Audit, serveFunc func(http.ResponseWriter, *http.Request)) http.Handler {
	u := &unauthenticatedHandler{
		serveFunc: serveFunc,
	}

	// if auditor is nil then return without wrapping
	if a == nil {
		return u
	}

	return a.WithUnauthorized(u)
}

func (u *unauthenticatedHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	u.serveFunc(rw, r)
}
