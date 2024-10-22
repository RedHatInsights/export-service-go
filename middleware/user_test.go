package middleware_test

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"

	chi "github.com/go-chi/chi/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/redhatinsights/export-service-go/middleware"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
)

var _ = Describe("Handler", func() {
	DescribeTable("Test EnforceUserIdentity middleware",
		func(accountNumber, orgID, username string, testIdentity string, expectedStatus int) {
			req, err := http.NewRequest("GET", "/test", nil)
			Expect(err).To(BeNil())

			// base64 encode the identity and add it to the request header "X-Rh-Identity"
			encodedIdentity := base64.StdEncoding.EncodeToString([]byte(testIdentity))

			req.Header.Add("X-Rh-Identity", encodedIdentity)

			handlerCalled := false

			rr := httptest.NewRecorder()
			applicationHandler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				// Check that the context has the UserIdentity field
				userIdentity := r.Context().Value(middleware.UserIdentityKey).(middleware.User)

				Expect(userIdentity.AccountID).To(Equal(accountNumber))
				Expect(userIdentity.OrganizationID).To(Equal(orgID))
				Expect(userIdentity.Username).To(Equal(username))

				handlerCalled = true
			})

			router := chi.NewRouter()
			router.Route("/", func(sub chi.Router) {
				sub.Use(identity.EnforceIdentity)
				sub.Use(middleware.EnforceUserIdentity)
				sub.Get("/test", applicationHandler)
			})

			router.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(expectedStatus))

			// Handler should not be called if an error is expected
			// The middleware would pass a bad context
			Expect(handlerCalled).To(Equal(expectedStatus == http.StatusOK))
		},
		Entry("Test with no header", nil, nil, nil, "", http.StatusBadRequest),
		Entry("Test with incorrect user type", "540155", "1979710", "username",
			`{ "identity": {"account_number": "540155", "auth_type": "jwt-auth", "org_id": "1979710", "internal": {"org_id": "1979710"}, "type": "Associate", "user": {"username": "username", "email": "boring@boring.mail.com", "first_name": "Jake", "last_name": "Logan", "is_active": true, "is_org_admin": false, "is_internal": true, "locale": "North America", "user_id": "1010101"} } }`,
			http.StatusBadRequest,
		),
		Entry("Test with valid context", "540155", "1979710", "username",
			`{ "identity": {"account_number": "540155", "auth_type": "jwt-auth", "org_id": "1979710", "internal": {"org_id": "1979710"}, "type": "User", "user": {"username": "username", "email": "boring@boring.mail.com", "first_name": "Jake", "last_name": "Logan", "is_active": true, "is_org_admin": false, "is_internal": true, "locale": "North America", "user_id": "1010101"} } }`,
			http.StatusOK,
		),
		Entry("Test with valid service account", "540155", "1979710", "service-account-username",
			`{ "identity": {"account_number": "540155", "auth_type": "jwt-auth", "org_id": "1979710", "internal": {"org_id": "1979710"}, "type": "ServiceAccount", "service_account": { "client_id": "b69eaf9e-e6a6-4f9e-805e-02987daddfbd", "username": "service-account-username" } } }`,
			http.StatusOK,
		),
		Entry("Test with empty service account (handle the null)", "540155", "1979710", "service-account-username",
			`{ "identity": {"account_number": "540155", "auth_type": "jwt-auth", "org_id": "1979710", "internal": {"org_id": "1979710"}, "type": "ServiceAccount", "service_account":  } }`,
			http.StatusBadRequest,
		),
		Entry("Test with service account with empty username", "540155", "1979710", "service-account-username",
			`{ "identity": {"account_number": "540155", "auth_type": "jwt-auth", "org_id": "1979710", "internal": {"org_id": "1979710"}, "type": "ServiceAccount", "service_account": { "client_id": "b69eaf9e-e6a6-4f9e-805e-02987daddfbd", "username": null } } }`,
			http.StatusBadRequest,
		),
		Entry("Test without org_id", "540155", "", "username",
			`{ "identity": {"account_number": "540155", "auth_type": "jwt-auth", "internal": {}, "type": "User", "user": {"username": "username", "email": "boring@boring.mail.com", "first_name": "Jake", "last_name": "Logan", "is_active": true, "is_org_admin": false, "is_internal": true, "locale": "North America", "user_id": "1010101"} } }`,
			http.StatusBadRequest,
		),
		Entry("Test with valid cert auth", "540155", "1979710", "deadbeef-e6a6-4f9e-805e-02987daddfbd",
			`{ "identity": {"account_number": "540155", "auth_type": "cert-auth", "org_id": "1979710", "internal": {"org_id": "1979710"}, "type": "System", "system": { "cn": "deadbeef-e6a6-4f9e-805e-02987daddfbd" } } }`,
			http.StatusOK,
		),
		Entry("Test cert auth with nil system object (handle the null)", "540155", "1979710", "ignore-me-user",
			`{ "identity": {"account_number": "540155", "auth_type": "cert-auth", "org_id": "1979710", "internal": {"org_id": "1979710"}, "type": "System", "system":  } }`,
			http.StatusBadRequest,
		),
		Entry("Test cert auth with empty cn", "540155", "1979710", "ignore-me-user",
			`{ "identity": {"account_number": "540155", "auth_type": "cert-auth", "org_id": "1979710", "internal": {"org_id": "1979710"}, "type": "System", "system": { "cn": ""} } }`,
			http.StatusBadRequest,
		),
		Entry("Test cert auth with null cn", "540155", "1979710", "ignore-me-user",
			`{ "identity": {"account_number": "540155", "auth_type": "cert-auth", "org_id": "1979710", "internal": {"org_id": "1979710"}, "type": "System", "system": { "cn": null } } }`,
			http.StatusBadRequest,
		),
	)
})
