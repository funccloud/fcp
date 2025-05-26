package tokenreview

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"go.funccloud.dev/fcp/internal/proxy/proxyutil"
	authv1 "k8s.io/api/authentication/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TokenReviewerInterface defines the contract for reviewing tokens.
type TokenReviewerInterface interface {
	Review(req *http.Request) (passthrough bool, err error)
}

var (
	timeout = time.Second * 10
)

type TokenReview struct {
	client    client.Client
	audiences []string
}

func New(client client.Client, audiences []string) *TokenReview {
	return &TokenReview{
		client:    client,
		audiences: audiences,
	}
}

func (t *TokenReview) Review(req *http.Request) (bool, error) {
	token, ok := proxyutil.ParseTokenFromRequest(req)
	if !ok {
		return false, errors.New("bearer token not found in request")
	}

	review := t.buildReview(token)

	ctx, cancel := context.WithTimeout(req.Context(), timeout)
	defer cancel()

	err := t.client.Create(ctx, review)
	if err != nil {
		return false, err
	}

	if len(review.Status.Error) > 0 {
		return false, fmt.Errorf("error authenticating using token review: %s",
			review.Status.Error)
	}

	return review.Status.Authenticated, nil
}

func (t *TokenReview) buildReview(token string) *authv1.TokenReview {
	return &authv1.TokenReview{
		Spec: authv1.TokenReviewSpec{
			Token:     token,
			Audiences: t.audiences,
		},
	}
}
