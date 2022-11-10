package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"

	chi "github.com/go-chi/chi/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/redhatinsights/export-service-go/models"
	"github.com/redhatinsights/platform-go-middlewares/identity"
)

var _ = Describe("Handler", func() {
	DescribeTable("Test EnforceUserIdentity middleware",
		func(useContext bool, userType, accountNumber, orgID, username string, expectedStatus int) {
			req, err := http.NewRequest("GET", "/test", nil)
			Expect(err).To(BeNil())

			testIdentity := identity.XRHID{
				Identity: identity.Identity{
					Type:          userType,
					AccountNumber: accountNumber,
					OrgID:         orgID,
					User: identity.User{
						Username: username,
					},
				},
			}

			if useContext {
				req = req.WithContext(context.WithValue(req.Context(), identity.Key, testIdentity))
			}

			handlerCalled := false

			rr := httptest.NewRecorder()
			applicationHandler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				// Check that the context has the UserIdentity field
				userIdentity := r.Context().Value(UserIdentityKey).(models.User)

				Expect(userIdentity.AccountID).To(Equal(accountNumber))
				Expect(userIdentity.OrganizationID).To(Equal(orgID))
				Expect(userIdentity.Username).To(Equal(username))

				handlerCalled = true
			})

			router := chi.NewRouter()
			router.Route("/", func(sub chi.Router) {
				sub.Use(EnforceUserIdentity)
				sub.Get("/test", applicationHandler)
			})

			router.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(expectedStatus))

			// Handler should not be called if an error is expected
			// The middleware would pass a bad context
			Expect(handlerCalled).To(Equal(expectedStatus == http.StatusOK))
		},
		Entry("Test with no context", false, nil, nil, nil, nil, http.StatusBadRequest),
		Entry("Test with valid context", true, "Associate", "11110000", "orgID", "username", http.StatusBadRequest),
		Entry("Test with valid context", true, "User", "11110000", "orgID", "username", http.StatusOK),
	)
})
