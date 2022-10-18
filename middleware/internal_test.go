package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/redhatinsights/identity"
)

var (
	IDENTITY = "eyJpZGVudGl0eSI6IHsiYWNjb3VudF9udW1iZXIiOiJhY2NvdW50MTIzIiwib3JnX2lkIjoib3JnMTIzIiwidHlwZSI6IlVzZXIiLCJ1c2VyIjp7ImlzX29yZ19hZG1pbiI6dHJ1ZX0sImludGVybmFsIjp7Im9yZ19pZCI6Im9yZzEyMyJ9fX0K"
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
			req, err := http.NewRequest("GET", fmt.Sprintf("/app/export/v1/%s/%s/%s", tc.ExportUUID, tc.Application, tc.ResourceUUID), nil)
			if err != nil {
				t.Fatal(err)
			}

			req.Header = make(http.Header)
			req.Header.Add("x-rh-identity", IDENTITY)
			req.Header.Add("x-rh-exports-a-psk", "testing-a-psk")
			req.Header.Add("Content-Type", "application/zip")

			rr := httptest.NewRecorder()

			applicationHandler := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				// handler should return 200
				rw.WriteHeader(http.StatusOK)
			})

			handler := identity.Extractor(URLParamsCtx(applicationHandler))

			handler.ServeHTTP(rr, req)

			if status := rr.Code; status != tc.ExpectedStatus {
				t.Errorf("handler returned wrong status code: got %v want %v", status, tc.ExpectedStatus)
			}
		})
	}
}
