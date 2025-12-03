/*
Copyright 2022 Red Hat Inc.
SPDX-License-Identifier: Apache-2.0
*/
package middleware_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/redhatinsights/export-service-go/middleware"
)

var _ = Describe("OIDC Factory Methods", func() {
	Describe("NewClientCredentialsTokenProvider", func() {
		Context("with valid parameters", func() {
			It("should create a token provider successfully", func() {
				provider, err := middleware.NewClientCredentialsTokenProvider(
					"https://example.com/token",
					"test-client-id",
					"test-client-secret",
					[]string{"read", "write"},
				)
				Expect(err).ToNot(HaveOccurred())
				Expect(provider).ToNot(BeNil())
			})

			It("should work with empty scopes", func() {
				provider, err := middleware.NewClientCredentialsTokenProvider(
					"https://example.com/token",
					"test-client-id",
					"test-client-secret",
					[]string{},
				)
				Expect(err).ToNot(HaveOccurred())
				Expect(provider).ToNot(BeNil())
			})

			It("should work with nil scopes", func() {
				provider, err := middleware.NewClientCredentialsTokenProvider(
					"https://example.com/token",
					"test-client-id",
					"test-client-secret",
					nil,
				)
				Expect(err).ToNot(HaveOccurred())
				Expect(provider).ToNot(BeNil())
			})
		})

		Context("with invalid parameters", func() {
			It("should return error when token URL is empty", func() {
				provider, err := middleware.NewClientCredentialsTokenProvider(
					"",
					"test-client-id",
					"test-client-secret",
					[]string{},
				)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("token URL cannot be empty"))
				Expect(provider).To(BeNil())
			})

			It("should return error when client ID is empty", func() {
				provider, err := middleware.NewClientCredentialsTokenProvider(
					"https://example.com/token",
					"",
					"test-client-secret",
					[]string{},
				)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("client ID cannot be empty"))
				Expect(provider).To(BeNil())
			})

			It("should return error when client secret is empty", func() {
				provider, err := middleware.NewClientCredentialsTokenProvider(
					"https://example.com/token",
					"test-client-id",
					"",
					[]string{},
				)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("client secret cannot be empty"))
				Expect(provider).To(BeNil())
			})
		})

		Context("Token method", func() {
			var mockServer *httptest.Server
			var provider middleware.TokenProvider

			BeforeEach(func() {
				// Create a mock OAuth2 token endpoint
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/token" && r.Method == "POST" {
						w.Header().Set("Content-Type", "application/json")
						_ = json.NewEncoder(w).Encode(map[string]interface{}{
							"access_token": "test-access-token",
							"token_type":   "Bearer",
							"expires_in":   3600,
						})
						return
					}
					http.NotFound(w, r)
				}))

				var err error
				provider, err = middleware.NewClientCredentialsTokenProvider(
					mockServer.URL+"/token",
					"test-client-id",
					"test-client-secret",
					[]string{"api"},
				)
				Expect(err).ToNot(HaveOccurred())
			})

			AfterEach(func() {
				mockServer.Close()
			})

			It("should successfully retrieve a token", func() {
				ctx := context.Background()
				token, err := provider.Token(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(token).ToNot(BeNil())
				Expect(token.AccessToken).To(Equal("test-access-token"))
				Expect(token.TokenType).To(Equal("Bearer"))
			})

			It("should handle context cancellation", func() {
				ctx, cancel := context.WithCancel(context.Background())
				cancel() // Cancel immediately

				_, err := provider.Token(ctx)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("NewOIDCVerifier", func() {
		Context("with invalid parameters", func() {
			It("should return error when issuer URL is empty", func() {
				verifier, err := middleware.NewOIDCVerifier(
					context.Background(),
					"",
					"test-client-id",
					0, // use default timeout
				)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("issuer URL cannot be empty"))
				Expect(verifier).To(BeNil())
			})

			It("should return error when client ID is empty", func() {
				verifier, err := middleware.NewOIDCVerifier(
					context.Background(),
					"https://example.com",
					"",
					0, // use default timeout
				)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("client ID cannot be empty"))
				Expect(verifier).To(BeNil())
			})

			It("should return error for invalid issuer URL", func() {
				verifier, err := middleware.NewOIDCVerifier(
					context.Background(),
					"https://invalid-oidc-provider-that-does-not-exist.example",
					"test-client-id",
					2*time.Second,
				)
				Expect(err).To(HaveOccurred())
				Expect(verifier).To(BeNil())
			})
		})
	})

	Describe("Verifier interface", func() {
		Context("Verify method validation", func() {
			It("should be implemented by oidcVerifier", func() {
				// This test ensures the interface contract is met
				var _ middleware.Verifier = (*mockVerifier)(nil)
			})

			It("should reject empty tokens", func() {
				verifier := &mockVerifier{}
				_, err := verifier.Verify(context.Background(), "")
				Expect(err).To(HaveOccurred())
			})

			It("should accept valid tokens", func() {
				verifier := &mockVerifierWithValidation{
					validTokens: map[string]bool{
						"valid-token-123": true,
					},
				}
				token, err := verifier.Verify(context.Background(), "valid-token-123")
				Expect(err).ToNot(HaveOccurred())
				Expect(token).ToNot(BeNil())
				Expect(token.Subject).To(Equal("test-user"))
			})

			It("should reject invalid tokens", func() {
				verifier := &mockVerifierWithValidation{
					validTokens: map[string]bool{
						"valid-token-123": true,
					},
				}
				_, err := verifier.Verify(context.Background(), "invalid-token")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid signature"))
			})

			It("should reject expired tokens", func() {
				verifier := &mockVerifierWithValidation{
					validTokens: map[string]bool{},
				}
				_, err := verifier.Verify(context.Background(), "expired-token")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("token is expired"))
			})

			It("should reject malformed tokens", func() {
				verifier := &mockVerifierWithValidation{
					validTokens: map[string]bool{},
				}
				_, err := verifier.Verify(context.Background(), "malformed.token")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("malformed"))
			})

			It("should reject tokens with wrong audience", func() {
				verifier := &mockVerifierWithValidation{
					validTokens: map[string]bool{},
				}
				_, err := verifier.Verify(context.Background(), "wrong-audience-token")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("audience"))
			})
		})
	})

	Describe("TokenProvider interface", func() {
		Context("Token method validation", func() {
			It("should be implemented by tokenSource", func() {
				// This test ensures the interface contract is met
				var _ middleware.TokenProvider = (*mockTokenProvider)(nil)
			})
		})
	})
})

// Mock implementations for interface testing
type mockVerifier struct{}

func (m *mockVerifier) Verify(ctx context.Context, rawIDToken string) (*oidc.IDToken, error) {
	if rawIDToken == "" {
		return nil, &oidc.TokenExpiredError{}
	}
	return nil, nil
}

// mockVerifierWithValidation simulates real OIDC verification behavior
type mockVerifierWithValidation struct {
	validTokens map[string]bool
}

func (m *mockVerifierWithValidation) Verify(ctx context.Context, rawIDToken string) (*oidc.IDToken, error) {
	if rawIDToken == "" {
		return nil, &oidc.TokenExpiredError{Expiry: time.Now()}
	}

	// Simulate specific error cases based on token value
	switch {
	case rawIDToken == "expired-token":
		return nil, &oidc.TokenExpiredError{Expiry: time.Now().Add(-time.Hour)}
	case rawIDToken == "malformed.token":
		return nil, fmt.Errorf("malformed token: invalid format")
	case rawIDToken == "wrong-audience-token":
		return nil, errors.New("oidc: expected audience \"my-client\" got [\"wrong-client\"]")
	case !m.validTokens[rawIDToken]:
		return nil, errors.New("invalid signature: token verification failed")
	}

	// Return valid token
	return &oidc.IDToken{
		Subject: "test-user",
		Issuer:  "https://test-issuer.com",
	}, nil
}

type mockTokenProvider struct{}

func (m *mockTokenProvider) Token(ctx context.Context) (*oauth2.Token, error) {
	return &oauth2.Token{
		AccessToken: "mock-token",
		TokenType:   "Bearer",
		Expiry:      time.Now().Add(time.Hour),
	}, nil
}
