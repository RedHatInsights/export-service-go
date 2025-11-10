package middleware

import (
	"context"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

// TokenProvider retrieves OAuth2 tokens for service-to-service authentication.
type TokenProvider interface {
	Token(ctx context.Context) (*oauth2.Token, error)
}

// Verifier verifies ID tokens from an OIDC provider.
type Verifier interface {
	Verify(ctx context.Context, rawIDToken string) (*oidc.IDToken, error)
}

// tokenSource implements TokenProvider using OAuth2 client credentials flow.
type tokenSource struct {
	config *clientcredentials.Config
}

// oidcVerifier implements Verifier using an OIDC ID token verifier.
type oidcVerifier struct {
	verifier *oidc.IDTokenVerifier
}

func NewClientCredentialsTokenProvider(tokenURL, clientID, clientSecret string, scopes []string) (TokenProvider, error) {
	if tokenURL == "" {
		return nil, fmt.Errorf("token URL cannot be empty")
	}
	if clientID == "" {
		return nil, fmt.Errorf("client ID cannot be empty")
	}
	if clientSecret == "" {
		return nil, fmt.Errorf("client secret cannot be empty")
	}

	config := &clientcredentials.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TokenURL:     tokenURL,
		Scopes:       scopes,
	}

	return &tokenSource{config: config}, nil
}

func NewOIDCVerifier(ctx context.Context, issuerURL, clientID string) (Verifier, error) {
	if issuerURL == "" {
		return nil, fmt.Errorf("issuer URL cannot be empty")
	}
	if clientID == "" {
		return nil, fmt.Errorf("client ID cannot be empty")
	}

	provider, err := oidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create OIDC provider: %w", err)
	}

	verifier := provider.Verifier(&oidc.Config{
		ClientID: clientID,
	})

	return &oidcVerifier{verifier: verifier}, nil
}

// Token retrieves an OAuth2 access token using the client credentials flow.
// The token is cached and automatically refreshed when expired.
func (t *tokenSource) Token(ctx context.Context) (*oauth2.Token, error) {
	tokenSource := t.config.TokenSource(ctx)
	token, err := tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to obtain access token: %w", err)
	}
	return token, nil
}

// Verify validates an ID token and returns the verified token with claims.
func (v *oidcVerifier) Verify(ctx context.Context, rawIDToken string) (*oidc.IDToken, error) {
	if rawIDToken == "" {
		return nil, fmt.Errorf("token cannot be empty")
	}

	token, err := v.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("token verification failed: %w", err)
	}
	return token, nil
}
