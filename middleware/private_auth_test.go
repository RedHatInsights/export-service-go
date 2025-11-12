/*
Copyright 2022 Red Hat Inc.
SPDX-License-Identifier: Apache-2.0
*/
package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/coreos/go-oidc/v3/oidc"

	"github.com/redhatinsights/export-service-go/middleware"
)

// mockVerifierForPrivateAuth simulates OIDC verification for private auth tests
type mockVerifierForPrivateAuth struct {
	shouldSucceed bool
	errorMessage  string
}

func (m *mockVerifierForPrivateAuth) Verify(ctx context.Context, rawIDToken string) (*oidc.IDToken, error) {
	if m.shouldSucceed {
		return &oidc.IDToken{
			Subject: "test-service",
			Issuer:  "https://test-issuer.com",
		}, nil
	}
	if m.errorMessage != "" {
		return nil, &oidc.TokenExpiredError{}
	}
	return nil, &oidc.TokenExpiredError{}
}

var _ = Describe("Private Authentication Middleware", func() {
	var (
		testHandler http.Handler
		psks        []string
	)

	BeforeEach(func() {
		psks = []string{"valid-psk-1", "valid-psk-2"}
		testHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("success"))
		})
	})

	Describe("EnforcePrivateAuth", func() {
		Context("with OIDC enabled and Bearer token present", func() {
			It("should authenticate successfully with valid Bearer token", func() {
				verifier := &mockVerifierForPrivateAuth{shouldSucceed: true}
				middleware := middleware.EnforcePrivateAuth(verifier, psks)
				handler := middleware(testHandler)

				req := httptest.NewRequest("GET", "/test", nil)
				req.Header.Set("Authorization", "Bearer valid-token")
				rec := httptest.NewRecorder()

				handler.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))
				Expect(rec.Body.String()).To(Equal("success"))
			})

			It("should reject invalid Bearer token", func() {
				verifier := &mockVerifierForPrivateAuth{shouldSucceed: false, errorMessage: "invalid"}
				middleware := middleware.EnforcePrivateAuth(verifier, psks)
				handler := middleware(testHandler)

				req := httptest.NewRequest("GET", "/test", nil)
				req.Header.Set("Authorization", "Bearer invalid-token")
				rec := httptest.NewRecorder()

				handler.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusUnauthorized))
				Expect(rec.Body.String()).To(ContainSubstring("OIDC authentication failed"))
			})

			It("should reject empty Bearer token", func() {
				verifier := &mockVerifierForPrivateAuth{shouldSucceed: true}
				middleware := middleware.EnforcePrivateAuth(verifier, psks)
				handler := middleware(testHandler)

				req := httptest.NewRequest("GET", "/test", nil)
				req.Header.Set("Authorization", "Bearer ")
				rec := httptest.NewRecorder()

				handler.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusUnauthorized))
				Expect(rec.Body.String()).To(ContainSubstring("empty bearer token"))
			})
		})

		Context("with PSK authentication (no Bearer token)", func() {
			It("should authenticate successfully with valid PSK", func() {
				verifier := &mockVerifierForPrivateAuth{shouldSucceed: true}
				middleware := middleware.EnforcePrivateAuth(verifier, psks)
				handler := middleware(testHandler)

				req := httptest.NewRequest("GET", "/test", nil)
				req.Header.Set("X-Rh-Exports-Psk", "valid-psk-1")
				rec := httptest.NewRecorder()

				handler.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))
				Expect(rec.Body.String()).To(Equal("success"))
			})

			It("should authenticate successfully with second valid PSK", func() {
				verifier := &mockVerifierForPrivateAuth{shouldSucceed: true}
				middleware := middleware.EnforcePrivateAuth(verifier, psks)
				handler := middleware(testHandler)

				req := httptest.NewRequest("GET", "/test", nil)
				req.Header.Set("X-Rh-Exports-Psk", "valid-psk-2")
				rec := httptest.NewRecorder()

				handler.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))
				Expect(rec.Body.String()).To(Equal("success"))
			})

			It("should reject invalid PSK", func() {
				verifier := &mockVerifierForPrivateAuth{shouldSucceed: true}
				middleware := middleware.EnforcePrivateAuth(verifier, psks)
				handler := middleware(testHandler)

				req := httptest.NewRequest("GET", "/test", nil)
				req.Header.Set("X-Rh-Exports-Psk", "invalid-psk")
				rec := httptest.NewRecorder()

				handler.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusUnauthorized))
				Expect(rec.Body.String()).To(ContainSubstring("invalid x-rh-exports-psk header"))
			})

			It("should reject missing PSK header", func() {
				verifier := &mockVerifierForPrivateAuth{shouldSucceed: true}
				middleware := middleware.EnforcePrivateAuth(verifier, psks)
				handler := middleware(testHandler)

				req := httptest.NewRequest("GET", "/test", nil)
				rec := httptest.NewRecorder()

				handler.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusBadRequest))
				Expect(rec.Body.String()).To(ContainSubstring("missing x-rh-exports-psk header"))
			})

			It("should reject multiple PSK headers", func() {
				verifier := &mockVerifierForPrivateAuth{shouldSucceed: true}
				middleware := middleware.EnforcePrivateAuth(verifier, psks)
				handler := middleware(testHandler)

				req := httptest.NewRequest("GET", "/test", nil)
				req.Header.Add("X-Rh-Exports-Psk", "psk-1")
				req.Header.Add("X-Rh-Exports-Psk", "psk-2")
				rec := httptest.NewRecorder()

				handler.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusBadRequest))
				Expect(rec.Body.String()).To(ContainSubstring("missing x-rh-exports-psk header"))
			})
		})

		Context("with OIDC disabled (nil verifier)", func() {
			It("should fall back to PSK authentication", func() {
				middleware := middleware.EnforcePrivateAuth(nil, psks)
				handler := middleware(testHandler)

				req := httptest.NewRequest("GET", "/test", nil)
				req.Header.Set("X-Rh-Exports-Psk", "valid-psk-1")
				rec := httptest.NewRecorder()

				handler.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))
				Expect(rec.Body.String()).To(Equal("success"))
			})

			It("should ignore Bearer token when OIDC disabled", func() {
				middleware := middleware.EnforcePrivateAuth(nil, psks)
				handler := middleware(testHandler)

				req := httptest.NewRequest("GET", "/test", nil)
				req.Header.Set("Authorization", "Bearer some-token")
				req.Header.Set("X-Rh-Exports-Psk", "valid-psk-1")
				rec := httptest.NewRecorder()

				handler.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))
				Expect(rec.Body.String()).To(Equal("success"))
			})
		})

		Context("authentication priority", func() {
			It("should prefer OIDC over PSK when both present", func() {
				verifier := &mockVerifierForPrivateAuth{shouldSucceed: true}
				middleware := middleware.EnforcePrivateAuth(verifier, psks)
				handler := middleware(testHandler)

				req := httptest.NewRequest("GET", "/test", nil)
				req.Header.Set("Authorization", "Bearer valid-token")
				req.Header.Set("X-Rh-Exports-Psk", "valid-psk-1")
				rec := httptest.NewRecorder()

				handler.ServeHTTP(rec, req)

				// Should succeed using OIDC (Bearer token)
				Expect(rec.Code).To(Equal(http.StatusOK))
				Expect(rec.Body.String()).To(Equal("success"))
			})

			It("should not fall back to PSK if Bearer token is present but invalid", func() {
				verifier := &mockVerifierForPrivateAuth{shouldSucceed: false}
				middleware := middleware.EnforcePrivateAuth(verifier, psks)
				handler := middleware(testHandler)

				req := httptest.NewRequest("GET", "/test", nil)
				req.Header.Set("Authorization", "Bearer invalid-token")
				req.Header.Set("X-Rh-Exports-Psk", "valid-psk-1")
				rec := httptest.NewRecorder()

				handler.ServeHTTP(rec, req)

				// Should fail - no fallback when Bearer token present
				Expect(rec.Code).To(Equal(http.StatusUnauthorized))
				Expect(rec.Body.String()).To(ContainSubstring("OIDC authentication failed"))
			})
		})
	})

	Describe("EnforceOIDCOnly", func() {
		Context("with valid OIDC verifier", func() {
			It("should authenticate successfully with valid Bearer token", func() {
				verifier := &mockVerifierForPrivateAuth{shouldSucceed: true}
				middleware := middleware.EnforceOIDCOnly(verifier)
				handler := middleware(testHandler)

				req := httptest.NewRequest("GET", "/test", nil)
				req.Header.Set("Authorization", "Bearer valid-token")
				rec := httptest.NewRecorder()

				handler.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))
				Expect(rec.Body.String()).To(Equal("success"))
			})

			It("should reject invalid Bearer token", func() {
				verifier := &mockVerifierForPrivateAuth{shouldSucceed: false}
				middleware := middleware.EnforceOIDCOnly(verifier)
				handler := middleware(testHandler)

				req := httptest.NewRequest("GET", "/test", nil)
				req.Header.Set("Authorization", "Bearer invalid-token")
				rec := httptest.NewRecorder()

				handler.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusUnauthorized))
				Expect(rec.Body.String()).To(ContainSubstring("OIDC authentication failed"))
			})

			It("should reject request without Authorization header", func() {
				verifier := &mockVerifierForPrivateAuth{shouldSucceed: true}
				middleware := middleware.EnforceOIDCOnly(verifier)
				handler := middleware(testHandler)

				req := httptest.NewRequest("GET", "/test", nil)
				rec := httptest.NewRecorder()

				handler.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusUnauthorized))
				Expect(rec.Body.String()).To(ContainSubstring("missing or invalid Authorization header"))
			})

			It("should reject request with non-Bearer Authorization", func() {
				verifier := &mockVerifierForPrivateAuth{shouldSucceed: true}
				middleware := middleware.EnforceOIDCOnly(verifier)
				handler := middleware(testHandler)

				req := httptest.NewRequest("GET", "/test", nil)
				req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
				rec := httptest.NewRecorder()

				handler.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusUnauthorized))
				Expect(rec.Body.String()).To(ContainSubstring("missing or invalid Authorization header"))
			})

			It("should reject empty Bearer token", func() {
				verifier := &mockVerifierForPrivateAuth{shouldSucceed: true}
				middleware := middleware.EnforceOIDCOnly(verifier)
				handler := middleware(testHandler)

				req := httptest.NewRequest("GET", "/test", nil)
				req.Header.Set("Authorization", "Bearer ")
				rec := httptest.NewRecorder()

				handler.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusUnauthorized))
				Expect(rec.Body.String()).To(ContainSubstring("empty bearer token"))
			})

			It("should not accept PSK when OIDC-only is enforced", func() {
				verifier := &mockVerifierForPrivateAuth{shouldSucceed: true}
				middleware := middleware.EnforceOIDCOnly(verifier)
				handler := middleware(testHandler)

				req := httptest.NewRequest("GET", "/test", nil)
				req.Header.Set("X-Rh-Exports-Psk", "valid-psk-1")
				rec := httptest.NewRecorder()

				handler.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusUnauthorized))
				Expect(rec.Body.String()).To(ContainSubstring("missing or invalid Authorization header"))
			})
		})

		Context("with nil verifier", func() {
			It("should return 500 when verifier is not configured", func() {
				middleware := middleware.EnforceOIDCOnly(nil)
				handler := middleware(testHandler)

				req := httptest.NewRequest("GET", "/test", nil)
				req.Header.Set("Authorization", "Bearer valid-token")
				rec := httptest.NewRecorder()

				handler.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusInternalServerError))
				Expect(rec.Body.String()).To(ContainSubstring("OIDC authentication is not configured"))
			})
		})
	})
})
