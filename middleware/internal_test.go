package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	chi "github.com/go-chi/chi/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Handler", func() {
	var (
		validUUID   = "0dc924db-20a3-415a-ae63-3434dd3e4f6a"
		invalidUUID = "1234"
		validApp    = "app"
	)

	DescribeTable("Test that invalid uuid do not make it into internal endpoints",
		func(exportUUID, application, resourceUUID string, expectedStatus int) {
			req, err := http.NewRequest("GET", fmt.Sprintf("/app/export/v1/%s/%s/%s/test", exportUUID, application, resourceUUID), nil)
			Expect(err).ToNot(HaveOccurred())

			rr := httptest.NewRecorder()

			handlerCalled := false

			applicationHandler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				rw.WriteHeader(http.StatusOK)
				handlerCalled = true
			})

			router := chi.NewRouter()
			router.Route("/app/export/v1/{exportUUID}/{application}/{resourceUUID}", func(sub chi.Router) {
				sub.Use(URLParamsCtx)
				sub.Get("/test", applicationHandler)
			})

			router.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(expectedStatus))

			// Handler should not be called if an error is expected
			// The middleware would pass a bad context
			Expect(handlerCalled).To(Equal(expectedStatus == http.StatusOK))
		},
		Entry("Both Invalid ExportUUID and ResourceUUID", invalidUUID, validApp, invalidUUID, http.StatusBadRequest),
		Entry("Invalid ExportUUID", invalidUUID, validApp, validUUID, http.StatusBadRequest),
		Entry("Invalid ResourceUUID", validUUID, validApp, invalidUUID, http.StatusBadRequest),
		Entry("Valid UUIDs", validUUID, validApp, validUUID, http.StatusOK),
	)
})
