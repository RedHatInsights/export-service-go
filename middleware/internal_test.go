package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	chi "github.com/go-chi/chi/v5"
)

// Test that invalid uuid do not make it into internal endpoints
func TestInternalMiddleware(t *testing.T) {
	var (
		valid_uuid   = "0dc924db-20a3-415a-ae63-3434dd3e4f6a"
		invalid_uuid = "1234"
		valid_app    = "app"
	)

	testCases := []struct {
		TestType       string
		ExportUUID     string
		Application    string
		ResourceUUID   string
		ExpectedStatus int
	}{
		{
			"Both Invalid ExportUUID and ResourceUUID",
			invalid_uuid,
			valid_app,
			invalid_uuid,
			http.StatusBadRequest,
		},
		{
			"Invalid ExportUUID",
			invalid_uuid,
			valid_app,
			valid_uuid,
			http.StatusBadRequest,
		},
		{
			"Invalid ResourceUUID",
			valid_uuid,
			valid_app,
			invalid_uuid,
			http.StatusBadRequest,
		},
		{
			"Valid UUIDs",
			valid_uuid,
			valid_app,
			valid_uuid,
			http.StatusOK,
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf(tc.TestType), func(t *testing.T) {
			req, err := http.NewRequest("GET", fmt.Sprintf("/app/export/v1/%s/%s/%s/test", tc.ExportUUID, tc.Application, tc.ResourceUUID), nil)
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()

			applicationHandler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				rw.WriteHeader(http.StatusOK)
			})

			router := chi.NewRouter()
			router.Route("/app/export/v1/{exportUUID}/{application}/{resourceUUID}", func(sub chi.Router) {
				sub.Use(URLParamsCtx)
				sub.Get("/test", applicationHandler)
			})

			router.ServeHTTP(rr, req)

			if status := rr.Code; status != tc.ExpectedStatus {
				t.Errorf("Handler returned wrong status code: got %v want %v", status, tc.ExpectedStatus)
			}
		})
	}
}
