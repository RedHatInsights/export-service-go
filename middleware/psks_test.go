package middleware_test

import (
	"net/http"
	"net/http/httptest"

	chi "github.com/go-chi/chi/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	config "github.com/redhatinsights/export-service-go/config"
	"github.com/redhatinsights/export-service-go/middleware"
)

var validExportConfig = &config.ExportConfig{
	Psks: []string{"test-psk"},
}

var _ = Describe("Handler", func() {
	DescribeTable("Test EnforcePSK function",
		func(useHeader, useMultipleHeaders bool, header string, expectedStatus int) {
			middleware.Cfg = validExportConfig

			req, err := http.NewRequest("GET", "/test", nil)
			Expect(err).To(BeNil())

			if useHeader {
				req.Header.Set("X-Rh-Exports-Psk", header)
			}

			if useMultipleHeaders {
				headerArray := []string{"1st-psk", "2nd-psk"}
				req.Header["X-Rh-Exports-Psk"] = headerArray
			}

			handlerCalled := false

			rr := httptest.NewRecorder()
			applicationHandler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				handlerCalled = true
			})

			router := chi.NewRouter()
			router.Route("/", func(sub chi.Router) {
				sub.Use(middleware.EnforcePSK)
				sub.Get("/test", applicationHandler)
			})

			router.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(expectedStatus))
			Expect(handlerCalled).To(Equal(expectedStatus == http.StatusOK))
		},
		Entry("Test with no header", false, false, nil, http.StatusBadRequest),
		Entry("Test with multiple headers", true, true, "", http.StatusBadRequest),
		Entry("Test with nil header", true, false, nil, http.StatusUnauthorized),
		Entry("Test with invalid header", true, false, "invalid", http.StatusUnauthorized),
		Entry("Test with valid header", true, false, validExportConfig.Psks[0], http.StatusOK),
	)

	DescribeTable("Test EnforceAppBoundPSK function",
		func(useHeader bool, header string, application string, expectedStatus int) {
			pskMap := map[string]string{
				"subscriptions":                        "subs-psk-secret",
				"urn:redhat:application:inventory":     "inv-psk-secret",
			}
			appBoundMiddleware := middleware.EnforceAppBoundPSK(pskMap)

			exportUUID := "550e8400-e29b-41d4-a716-446655440000"
			resourceUUID := "660e8400-e29b-41d4-a716-446655440000"
			url := "/" + exportUUID + "/" + application + "/" + resourceUUID + "/upload"
			req, err := http.NewRequest("POST", url, nil)
			Expect(err).To(BeNil())

			if useHeader {
				req.Header.Set("X-Rh-Exports-Psk", header)
			}

			handlerCalled := false

			rr := httptest.NewRecorder()
			applicationHandler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				handlerCalled = true
			})

			router := chi.NewRouter()
			router.Route("/{exportUUID}/{application}/{resourceUUID}", func(sub chi.Router) {
				sub.Use(middleware.URLParamsCtx)
				sub.Use(appBoundMiddleware)
				sub.Post("/upload", applicationHandler)
			})

			router.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(expectedStatus))
			Expect(handlerCalled).To(Equal(expectedStatus == http.StatusOK))
		},
		Entry("Valid PSK for correct app", true, "subs-psk-secret", "subscriptions", http.StatusOK),
		Entry("Valid PSK for wrong app", true, "subs-psk-secret", "urn:redhat:application:inventory", http.StatusUnauthorized),
		Entry("No PSK configured for app", true, "subs-psk-secret", "unknown-app", http.StatusForbidden),
		Entry("Missing header", false, "", "subscriptions", http.StatusBadRequest),
		Entry("Invalid PSK", true, "wrong-psk", "subscriptions", http.StatusUnauthorized),
	)
})
