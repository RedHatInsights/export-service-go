package middleware_test

import (
	"net/http"
	"net/http/httptest"

	chi "github.com/go-chi/chi/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	// exportconfig from the config package
	config "github.com/redhatinsights/export-service-go/config"
	"github.com/redhatinsights/export-service-go/middleware"
)

var (
	validExportConfig = &config.ExportConfig{
		Psks: []string{"test-psk"},
	}
)
var _ = Describe("Handler", func() {
	DescribeTable("Test EnforcePSK function",
		func(useHeader, useMultipleHeaders bool, header string, expectedStatus int) {
			// set the user's config to validExportConfig
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
				// The EnforcePsk function does nothing to the context here

				handlerCalled = true
			})

			router := chi.NewRouter()
			router.Route("/", func(sub chi.Router) {
				sub.Use(middleware.EnforcePSK)
				sub.Get("/test", applicationHandler)
			})

			router.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(expectedStatus))

			// Handler should not be called if an error is expected
			// The middleware would pass a bad context
			Expect(handlerCalled).To(Equal(expectedStatus == http.StatusOK))
		},
		Entry("Test with no header", false, false, nil, http.StatusBadRequest),
		Entry("Test with multiple headers", true, true, "", http.StatusBadRequest),
		Entry("Test with nil header", true, false, nil, http.StatusUnauthorized),
		Entry("Test with invalid header", true, false, "invalid", http.StatusUnauthorized),
		Entry("Test with valid header", true, false, validExportConfig.Psks[0], http.StatusOK),
	)
})
