package tokenreview

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	authv1 "k8s.io/api/authentication/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/authentication"
	"sigs.k8s.io/yaml"
)

func SetupTokenReviewWebhookWithManager(mgr ctrl.Manager, userInfoEndpoint string) error {
	if userInfoEndpoint == "" {
		return fmt.Errorf("userinfo-endpoint is required")
	}
	mgr.GetWebhookServer().Register("/", &authentication.Webhook{
		Handler: &Authenticator{
			UserInfoEndpoint: userInfoEndpoint,
		},
	})
	return nil
}

// Authenticator validates tokenreviews
type Authenticator struct {
	// UserInfoEndpoint is the endpoint to use for authentication.
	UserInfoEndpoint string
}

// authenticator admits a request by the token.
func (a *Authenticator) Handle(ctx context.Context, r authentication.Request) authentication.Response {
	client := http.Client{
		Timeout: 10 * time.Second,
	}
	l := log.FromContext(ctx)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.UserInfoEndpoint, nil)
	if err != nil {
		return authentication.Errored(err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", r.Spec.Token))
	resp, err := client.Do(req)
	if err != nil {
		return authentication.Errored(err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			l.Error(closeErr, "failed to close response body")
		}
	}()
	byt, err := io.ReadAll(resp.Body)
	if err != nil {
		return authentication.Errored(err)
	}
	if resp.StatusCode != http.StatusOK {
		return authentication.Unauthenticated(string(byt), authv1.UserInfo{})
	}
	userInfo := authv1.UserInfo{}
	if err := yaml.Unmarshal(byt, &userInfo); err != nil {
		return authentication.Errored(err)
	}
	return authentication.Authenticated("", userInfo)
}
