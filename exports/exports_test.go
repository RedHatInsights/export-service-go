package exports_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/redhatinsights/platform-go-middlewares/v2/identity"

	"github.com/redhatinsights/export-service-go/config"
	"github.com/redhatinsights/export-service-go/exports"
	"github.com/redhatinsights/export-service-go/logger"
	emiddleware "github.com/redhatinsights/export-service-go/middleware"
	"github.com/redhatinsights/export-service-go/models"
	es3 "github.com/redhatinsights/export-service-go/s3"
)

const debugHeader string = "eyJpZGVudGl0eSI6eyJhY2NvdW50X251bWJlciI6IjEwMDAxIiwib3JnX2lkIjoiMTAwMDAwMDEiLCJpbnRlcm5hbCI6eyJvcmdfaWQiOiIxMDAwMDAwMSJ9LCJ0eXBlIjoiVXNlciIsInVzZXIiOnsidXNlcm5hbWUiOiJ1c2VyX2RldiJ9fX0="
const formatDateTime string = "2006-01-02T15:04:05Z" // ISO 8601

func AddDebugUserIdentity(req *http.Request) {
	req.Header.Add("x-rh-identity", debugHeader)
}

func generateExportRequestBody(name, format, expires, sources string) (exportRequest []byte) {
	if expires != "" {
		return []byte(fmt.Sprintf(`{"name": "%s", "format": "%s", "expires_at": "%s", "sources": [%s]}`, name, format, expires, sources))
	}
	if format == "" {
		return []byte(fmt.Sprintf(`{"name": "%s", "sources": [%s]}`, name, sources))
	}
	return []byte(fmt.Sprintf(`{"name": "%s", "format": "%s", "sources": [%s]}`, name, format, sources))
}

func createExportRequest(name, format, expires, sources string) *http.Request {
	exportRequest := generateExportRequestBody(name, format, expires, sources)
	request, err := http.NewRequest("POST", "/api/export/v1/exports", bytes.NewBuffer(exportRequest))
	Expect(err).To(BeNil())
	request.Header.Set("Content-Type", "application/json")
	return request
}

var _ = Describe("The public API", func() {
	DescribeTable("can create a new export request", func(name, format, expires, sources, expectedBody string, expectedStatus int) {
		router := setupTest(mockRequestApplicationResources)

		req := createExportRequest(name, format, expires, sources)

		rr := httptest.NewRecorder()

		AddDebugUserIdentity(req)
		router.ServeHTTP(rr, req)
		Expect(rr.Code).To(Equal(expectedStatus))
		Expect(rr.Body.String()).To(ContainSubstring(expectedBody))
	},
		Entry("with valid request", "Test Export Request", "json", "2023-01-01T00:00:00Z", `{"application":"exampleApp", "resource":"exampleResource"}`, "", http.StatusAccepted),
		Entry("with no expiration", "Test Export Request", "json", "", `{"application":"exampleApp", "resource":"exampleResource"}`, "", http.StatusAccepted),
		Entry("with an invalid format", "Test Export Request", "abcde", "2023-01-01T00:00:00Z", `{"application":"exampleApp", "resource":"exampleResource"}`, "invalid or missing payload format", http.StatusBadRequest),
		Entry("with a missing format", "Test Export Request", "", "2023-01-01T00:00:00Z", `{"application":"exampleApp", "resource":"exampleResource"}`, "invalid or missing payload format", http.StatusBadRequest),
		Entry("With no sources", "Test Export Request", "json", "2023-01-01T00:00:00Z", "", "no sources provided", http.StatusBadRequest),
	)

	It("can list all export requests", func() {
		router := setupTest(mockRequestApplicationResources)

		rr := httptest.NewRecorder()

		// Generate 3 export requests
		for i := 1; i <= 3; i++ {
			req := createExportRequest(
				fmt.Sprintf("Test Export Request %d", i),
				"json",
				"",
				`{"application":"exampleApp", "resource":"exampleResource"}`,
			)

			AddDebugUserIdentity(req)
			router.ServeHTTP(rr, req)
			Expect(rr.Code).To(Equal(http.StatusAccepted))
		}

		req, err := http.NewRequest("GET", "/api/export/v1/exports", nil)
		req.Header.Set("Content-Type", "application/json")
		Expect(err).ShouldNot(HaveOccurred())

		rr = httptest.NewRecorder()

		AddDebugUserIdentity(req)
		router.ServeHTTP(rr, req)
		Expect(rr.Code).To(Equal(http.StatusOK))
		Expect(rr.Body.String()).To(ContainSubstring("Test Export Request 1"))
		Expect(rr.Body.String()).To(ContainSubstring("Test Export Request 2"))
		Expect(rr.Body.String()).To(ContainSubstring("Test Export Request 3"))
	})

	DescribeTable("can filter and list export requests", func(filter string, expectedExports []string, expectedCount, expectedStatus int) {
		router := populateTestData()

		req, err := http.NewRequest("GET", fmt.Sprintf("/api/export/v1/exports?%s", filter), nil)
		req.Header.Set("Content-Type", "application/json")
		Expect(err).ShouldNot(HaveOccurred())

		rr := httptest.NewRecorder()

		AddDebugUserIdentity(req)
		router.ServeHTTP(rr, req)
		Expect(rr.Code).To(Equal(expectedStatus))
		for _, expectedExport := range expectedExports {
			Expect(rr.Body.String()).To(ContainSubstring(expectedExport))
		}
		Expect(strings.Count(rr.Body.String(), "id")).To(Equal(expectedCount))

		if expectedCount == 0 {
			// ensure that the data field is empty
			Expect(rr.Body.String()).To(ContainSubstring(`"data":[]`))
		}
	},
		Entry("by name", "name=Test Export Request 1", []string{"Test Export Request 1"}, 1, http.StatusOK),
		Entry("by status", "status=pending", []string{"Test Export Request 1", "Test Export Request 2", "Test Export Request 3", "Test Export Request 4", "Test Export Request 5", "Test Export Request 6"}, 6, http.StatusOK),
		Entry("by created at (given date)", "created_at=2021-01-01", []string{}, 0, http.StatusOK),
		Entry("by created at (given date-time)", "created_at=2021-01-01T00:00:00Z", []string{}, 0, http.StatusOK),
		Entry("by improper created at", "created_at=spring", []string{"'spring' is not a valid date in ISO 8601"}, 1, http.StatusBadRequest), // no exports returned, but the message is 1 count
		Entry("by expires", "expires_at=2023-01-01T00:00:00Z", []string{}, 0, http.StatusOK),
		Entry("by improper expires", "expires_at=nextyear", []string{"'nextyear' is not a valid date in ISO 8601"}, 1, http.StatusBadRequest),
		Entry("by application", "application=exampleApp", []string{"Test Export Request 1", "Test Export Request 2", "Test Export Request 3"}, 3, http.StatusOK),
		Entry("by resource", "resource=exampleResource2", []string{"Test Export Request 4", "Test Export Request 5", "Test Export Request 6"}, 3, http.StatusOK),
		Entry("by application and resource", "application=exampleApp3&resource=exampleResource2", []string{"Test Export Request 6"}, 1, http.StatusOK),
		Entry("by application that doesn't exist", "application=notAnApp", []string{}, 0, http.StatusOK),
		Entry("by resource that doesn't exist", "resource=notAResource", []string{}, 0, http.StatusOK),
		Entry("by application and resource that don't exist", "application=notAnApp&resource=notAResource", []string{}, 0, http.StatusOK),
		Entry("by application and resource combination that doesn't exist", "application=exampleApp&resource=exampleResource2", []string{}, 0, http.StatusOK),
	)

	Describe("can filter exports by date", func() {
		It("with created at in date format", func() {
			router := populateTestData() // check this function for logic on export creation

			rr := httptest.NewRecorder()

			today := time.Now().Format("2006-01-02")

			req, err := http.NewRequest("GET", fmt.Sprintf("/api/export/v1/exports?created_at=%s", today), nil)

			req.Header.Set("Content-Type", "application/json")

			Expect(err).ShouldNot(HaveOccurred())

			AddDebugUserIdentity(req)
			router.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusOK))
			// check the count of exports returned
			Expect(rr.Body.String()).To(ContainSubstring("count\":4"))
			Expect(rr.Body.String()).ToNot(ContainSubstring("Test Export Request 1"))
			Expect(rr.Body.String()).ToNot(ContainSubstring("Test Export Request 2"))
			Expect(rr.Body.String()).To(ContainSubstring("Test Export Request 3"))
			Expect(rr.Body.String()).To(ContainSubstring("Test Export Request 4"))
			Expect(rr.Body.String()).To(ContainSubstring("Test Export Request 5"))
			Expect(rr.Body.String()).To(ContainSubstring("Test Export Request 6"))
		})

		It("with created at in date-time format", func() {
			router := populateTestData()

			rr := httptest.NewRecorder()

			today := time.Now().UTC().Format(formatDateTime)

			req, err := http.NewRequest("GET", fmt.Sprintf("/api/export/v1/exports?created_at=%s", today), nil)

			req.Header.Set("Content-Type", "application/json")

			Expect(err).ShouldNot(HaveOccurred())

			AddDebugUserIdentity(req)
			router.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusOK))
			Expect(rr.Body.String()).To(ContainSubstring("count\":4"))
			Expect(rr.Body.String()).ToNot(ContainSubstring("Test Export Request 1"))
			Expect(rr.Body.String()).ToNot(ContainSubstring("Test Export Request 2"))
			Expect(rr.Body.String()).To(ContainSubstring("Test Export Request 3"))
			Expect(rr.Body.String()).To(ContainSubstring("Test Export Request 4"))
			Expect(rr.Body.String()).To(ContainSubstring("Test Export Request 5"))
			Expect(rr.Body.String()).To(ContainSubstring("Test Export Request 6"))
		})

		It("with created at referring to yesterday", func() {
			router := populateTestData()

			rr := httptest.NewRecorder()

			yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")

			req, err := http.NewRequest("GET", fmt.Sprintf("/api/export/v1/exports?created_at=%s", yesterday), nil)

			req.Header.Set("Content-Type", "application/json")

			Expect(err).ShouldNot(HaveOccurred())

			AddDebugUserIdentity(req)
			router.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusOK))
			Expect(rr.Body.String()).To(ContainSubstring("count\":1"))
			Expect(rr.Body.String()).To(ContainSubstring("Test Export Request 1"))
			Expect(rr.Body.String()).ToNot(ContainSubstring("Test Export Request 2"))
			Expect(rr.Body.String()).ToNot(ContainSubstring("Test Export Request 3"))
		})

		It("with expires in date format", func() {
			router := populateTestData()

			rr := httptest.NewRecorder()

			today := time.Now().AddDate(0, 0, 1).Format("2006-01-02")

			req, err := http.NewRequest("GET", fmt.Sprintf("/api/export/v1/exports?expires_at=%s", today), nil)

			req.Header.Set("Content-Type", "application/json")

			Expect(err).ShouldNot(HaveOccurred())

			AddDebugUserIdentity(req)
			router.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusOK))
			Expect(rr.Body.String()).To(ContainSubstring("count\":1"))
			Expect(rr.Body.String()).ToNot(ContainSubstring("Test Export Request 1"))
			Expect(rr.Body.String()).ToNot(ContainSubstring("Test Export Request 2"))
			Expect(rr.Body.String()).To(ContainSubstring("Test Export Request 3"))
		})
	})

	DescribeTable("can offset and limit exports", func(param string, expectedFirst, expectedLast string) {
		// make a large amount of data
		router := setupTest(mockRequestApplicationResources)

		count := 200

		for i := 1; i <= count; i++ {
			req := createExportRequest(
				fmt.Sprintf("Test Export Request %d", i),
				"json",
				"",
				`{"application":"exampleApp", "resource":"exampleResource"}`,
			)

			rr := httptest.NewRecorder()

			AddDebugUserIdentity(req)
			router.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusAccepted))
		}

		rr := httptest.NewRecorder()

		req, err := http.NewRequest("GET", fmt.Sprintf("/api/export/v1/exports?%s", param), nil)
		req.Header.Set("Content-Type", "application/json")
		Expect(err).ShouldNot(HaveOccurred())

		AddDebugUserIdentity(req)
		router.ServeHTTP(rr, req)

		Expect(rr.Code).To(Equal(http.StatusOK))
		Expect(rr.Body.String()).To(ContainSubstring(fmt.Sprintf("count\":%d", count)))
		Expect(rr.Body.String()).To(ContainSubstring(expectedFirst))
		Expect(rr.Body.String()).To(ContainSubstring(expectedLast))
	},
		Entry("offset 0, limit 10", "offset=0&limit=10", "Test Export Request 1", "Test Export Request 10"),
		Entry("offset 10, limit 10", "offset=10&limit=10", "Test Export Request 11", "Test Export Request 20"),
		Entry("offset 20, limit 10", "offset=20&limit=10", "Test Export Request 21", "Test Export Request 30"),
		Entry("offset 100, limit 10", "offset=100&limit=10", "Test Export Request 101", "Test Export Request 110"),
		Entry("offset 195, limit 10", "offset=195&limit=10", "Test Export Request 196", "Test Export Request 200"),
		Entry("offset 0, limit 200", "offset=0&limit=200", "Test Export Request 1", "Test Export Request 200"),
		Entry("offset over count, limit 200", "offset=1000&limit=200", "", ""),
		Entry("limit over count", "offset=0&limit=1000", "Test Export Request 1", "Test Export Request 200"),
	)

	It("with offset > count, returns empty data", func() {
		router := populateTestData()

		count := 6

		rr := httptest.NewRecorder()

		req, err := http.NewRequest("GET", "/api/export/v1/exports?offset=1000&limit=200", nil)

		req.Header.Set("Content-Type", "application/json")
		Expect(err).ShouldNot(HaveOccurred())

		AddDebugUserIdentity(req)
		router.ServeHTTP(rr, req)

		Expect(rr.Code).To(Equal(http.StatusOK))
		Expect(rr.Body.String()).To(ContainSubstring(fmt.Sprintf("count\":%d", count)))
		Expect(rr.Body.String()).To(ContainSubstring("data\":[]"))
	})

	DescribeTable("can sort exports", func(params string, expectedFirst, expectedSecond, expectedThird, expectedFourth, expectedLast string) {
		router := setupTest(mockRequestApplicationResources)

		count := 5

		for i := 1; i <= count; i++ {
			req := createExportRequest(
				fmt.Sprintf("Test Export Request %d", i),
				"json",
				"",
				`{"application":"exampleApp", "resource":"exampleResource"}`,
			)

			rr := httptest.NewRecorder()

			AddDebugUserIdentity(req)
			router.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusAccepted))
		}

		// ----------------
		// modify the created at time of the first export
		tenDaysFromNow := time.Now().AddDate(0, 0, 10)
		modifyExportCreated("Test Export Request 1", tenDaysFromNow)

		// modify the expires at time of the last export
		OneDayAgo := time.Now().AddDate(0, 0, -1)
		modifyExportExpires("Test Export Request 5", OneDayAgo)

		// ----------------

		rr := httptest.NewRecorder()

		req, err := http.NewRequest("GET", fmt.Sprintf("/api/export/v1/exports?%s", params), nil)

		Expect(err).ShouldNot(HaveOccurred())

		AddDebugUserIdentity(req)
		router.ServeHTTP(rr, req)

		Expect(rr.Code).To(Equal(http.StatusOK))

		expectedNames := []string{
			expectedFirst,
			expectedSecond,
			expectedThird,
			expectedFourth,
			expectedLast,
		}

		recieved := getExportNames(rr)

		for i, name := range expectedNames {
			Expect(name).To(Equal(recieved[i]))
		}

	},
		Entry("default of created asc", "",
			"Test Export Request 2",
			"Test Export Request 3",
			"Test Export Request 4",
			"Test Export Request 5",
			"Test Export Request 1",
		),
		Entry("sort by created asc", "sort=created&dir=asc",
			"Test Export Request 2",
			"Test Export Request 3",
			"Test Export Request 4",
			"Test Export Request 5",
			"Test Export Request 1",
		),
		Entry("sort by created desc", "sort=created&dir=desc",
			"Test Export Request 1",
			"Test Export Request 5",
			"Test Export Request 4",
			"Test Export Request 3",
			"Test Export Request 2",
		),
		Entry("sort by expires asc", "sort=expires&dir=asc",
			"Test Export Request 5",
			"Test Export Request 1",
			"Test Export Request 2",
			"Test Export Request 3",
			"Test Export Request 4",
		),
		Entry("sort by expires desc", "sort=expires&dir=desc",
			"Test Export Request 4",
			"Test Export Request 3",
			"Test Export Request 2",
			"Test Export Request 1",
			"Test Export Request 5",
		),
		Entry("sort by name asc", "sort=name&dir=asc",
			"Test Export Request 1",
			"Test Export Request 2",
			"Test Export Request 3",
			"Test Export Request 4",
			"Test Export Request 5",
		),
		Entry("sort by name desc", "sort=name&dir=desc",
			"Test Export Request 5",
			"Test Export Request 4",
			"Test Export Request 3",
			"Test Export Request 2",
			"Test Export Request 1",
		),
	)

	It("can check the status of an export request", func() {
		router := setupTest(mockRequestApplicationResources)

		rr := httptest.NewRecorder()

		req := createExportRequest(
			"Test Export Request",
			"json",
			"",
			`{"application":"exampleApp", "resource":"exampleResource"}`,
		)
		AddDebugUserIdentity(req)
		router.ServeHTTP(rr, req)
		Expect(rr.Code).To(Equal(http.StatusAccepted))

		// Grab the 'id' from the response
		var exportResponse map[string]interface{}
		err := json.Unmarshal(rr.Body.Bytes(), &exportResponse)
		Expect(err).ShouldNot(HaveOccurred())
		exportUUID := exportResponse["id"].(string)

		// Check the status of the export request
		req, err = http.NewRequest("GET", fmt.Sprintf("/api/export/v1/exports/%s/status", exportUUID), nil)
		req.Header.Set("Content-Type", "application/json")

		Expect(err).ShouldNot(HaveOccurred())

		rr = httptest.NewRecorder()

		AddDebugUserIdentity(req)
		router.ServeHTTP(rr, req)
		Expect(rr.Code).To(Equal(http.StatusOK))
		Expect(rr.Body.String()).To(ContainSubstring(`"status":"pending"`))
	})

	It("sends a request message to the export sources", func() {
		var wasKafkaMessageSent bool

		mockKafkaCall := func(ctx context.Context, log *zap.SugaredLogger, identity string, payload models.ExportPayload) {
			wasKafkaMessageSent = true
		}

		router := setupTest(mockKafkaCall)

		rr := httptest.NewRecorder()

		req := createExportRequest(
			"Test Export Request",
			"json",
			"",
			`{"application":"exampleApp", "resource":"exampleResource"}`,
		)

		AddDebugUserIdentity(req)
		router.ServeHTTP(rr, req)

		Expect(rr.Code).To(Equal(http.StatusAccepted))
		Expect(wasKafkaMessageSent).To(BeTrue())
	})

	// It("can get a completed export request by ID and download it")

	It("can delete a specific export request by ID", func() {
		router := setupTest(mockRequestApplicationResources)

		rr := httptest.NewRecorder()

		req := createExportRequest(
			"Test Export Request",
			"json",
			"",
			`{"application":"exampleApp", "resource":"exampleResource"}`,
		)
		AddDebugUserIdentity(req)
		router.ServeHTTP(rr, req)
		Expect(rr.Code).To(Equal(http.StatusAccepted))

		// Grab the 'id' from the export request
		var exportResponse map[string]interface{}
		err := json.Unmarshal(rr.Body.Bytes(), &exportResponse)
		Expect(err).ShouldNot(HaveOccurred())
		exportUUID := exportResponse["id"].(string)

		// Delete the export request
		req, err = http.NewRequest("DELETE", fmt.Sprintf("/api/export/v1/exports/%s", exportUUID), nil)
		req.Header.Set("Content-Type", "application/json")
		Expect(err).ShouldNot(HaveOccurred())

		AddDebugUserIdentity(req)
		router.ServeHTTP(rr, req)
		Expect(rr.Code).To(Equal(http.StatusAccepted))

		// Check that the export was deleted
		rr = httptest.NewRecorder()
		req, err = http.NewRequest("GET", fmt.Sprintf("/api/export/v1/exports/%s", exportUUID), nil)
		req.Header.Set("Content-Type", "application/json")
		Expect(err).ShouldNot(HaveOccurred())

		AddDebugUserIdentity(req)
		router.ServeHTTP(rr, req)
		Expect(rr.Code).To(Equal(http.StatusNotFound))
		Expect(rr.Body.String()).To(ContainSubstring("not found"))
	})

	It("returns the appropriate error if the format is missing", func() {
		router := setupTest(mockRequestApplicationResources)

		rr := httptest.NewRecorder()

		exportRequest := []byte(fmt.Sprintf(`{"name": "%s", "sources": [%s]}`, "Test Export Request", `{"application":"exampleApp", "resource":"exampleResource"}`))
		req, err := http.NewRequest("POST", "/api/export/v1/exports", bytes.NewBuffer(exportRequest))
		Expect(err).To(BeNil())
		req.Header.Set("Content-Type", "application/json")

		AddDebugUserIdentity(req)
		router.ServeHTTP(rr, req)
		Expect(rr.Code).To(Equal(http.StatusBadRequest))
		Expect(rr.Body.String()).To(ContainSubstring("invalid or missing payload format"))
	})
	
	It("returns the appropriate error if an application or resource is incorrect ", func() {
		router := setupTest(mockRequestApplicationResources)

		rr := httptest.NewRecorder()

		req := createExportRequest(
			"Test Export Request",
			"json",
			"",
			`{"application":"exampleApp", "resource":"exampleResource"}`,
		)
		AddDebugUserIdentity(req)
		router.ServeHTTP(rr, req)
		Expect(rr.Code).To(Equal(http.StatusNotAcceptable))
		Expect(rr.Body.String()).To(ContainSubstring("Payload does not match Configured Exports"))
	})
})

func mockRequestApplicationResources(ctx context.Context, log *zap.SugaredLogger, identity string, payload models.ExportPayload) {
	// fmt.Println("MOCKED !!  KAFKA SENT: TRUE ")
}

func setupTest(requestAppResources exports.RequestApplicationResources) chi.Router {
	var exportHandler *exports.Export
	var router *chi.Mux
	config := config.Get()
	log := logger.Get()

	fmt.Println("STARTING TEST")

	exportHandler = &exports.Export{
		Bucket:              "cfg.StorageConfig.Bucket",
		StorageHandler:      &es3.MockStorageHandler{},
		DB:                  &models.ExportDB{DB: testGormDB, Cfg: config},
		RequestAppResources: requestAppResources,
		Log:                 log,
	}

	router = chi.NewRouter()
	router.Use(
		identity.EnforceIdentity,
		emiddleware.EnforceUserIdentity,
	)

	router.Route("/api/export/v1", func(sub chi.Router) {
		sub.Post("/exports", exportHandler.PostExport)
		sub.With(emiddleware.PaginationCtx).Get("/exports", exportHandler.ListExports)
		sub.Get("/exports/{exportUUID}/status", exportHandler.GetExportStatus)
		sub.Delete("/exports/{exportUUID}", exportHandler.DeleteExport)
		sub.Get("/exports/{exportUUID}", exportHandler.GetExport)
	})

	fmt.Println("...CLEANING DB...")
	testGormDB.Exec("DELETE FROM export_payloads")

	return router
}

func populateTestData() chi.Router {
	// define router
	router := setupTest(mockRequestApplicationResources)

	for i := 1; i <= 6; i++ {
		req := createExportRequest(
			fmt.Sprintf("Test Export Request %d", i),
			"json",
			"",
			`{"application":"exampleApp", "resource":"exampleResource"}`,
		)

		rr := httptest.NewRecorder()

		AddDebugUserIdentity(req)
		router.ServeHTTP(rr, req)
	}

	oneDayAgo := time.Now().AddDate(0, 0, -1).UTC()
	oneDayFromNow := time.Now().AddDate(0, 0, 1).UTC()

	modifyExportCreated("Test Export Request 1", oneDayAgo)
	modifyExportCreated("Test Export Request 2", oneDayFromNow)
	modifyExportExpires("Test Export Request 3", oneDayFromNow)

	// Used for testing filtering by application and resource
	modifyExportSources("Test Export Request 4", "exampleApp2", "exampleResource2")
	modifyExportSources("Test Export Request 5", "exampleApp2", "exampleResource2")
	modifyExportSources("Test Export Request 6", "exampleApp3", "exampleResource2")

	return router
}

func modifyExportCreated(exportName string, newDate time.Time) {
	testGormDB.Exec("UPDATE export_payloads SET created_at = ? WHERE name = ?", newDate, exportName)
}

func modifyExportExpires(exportName string, newDate time.Time) {
	testGormDB.Exec("UPDATE export_payloads SET expires= ? WHERE name = ?", newDate, exportName)
}

// modify the application and resource of all sources in a given export
func modifyExportSources(exportName string, application string, resource string) {
	var exportPayload models.ExportPayload
	testGormDB.Where("name = ?", exportName).First(&exportPayload)

	var sources []models.Source
	testGormDB.Find(&sources)

	for _, source := range sources {
		if source.ExportPayloadID == exportPayload.ID {
			source.Application = application
			source.Resource = resource
			testGormDB.Save(&source)
		}
	}
}

func getExportNames(rr *httptest.ResponseRecorder) []string {
	b, err := io.ReadAll(rr.Body)
	Expect(err).ShouldNot(HaveOccurred())

	var exportResponse map[string]interface{}
	err = json.Unmarshal(b, &exportResponse)
	Expect(err).ShouldNot(HaveOccurred())

	var exportNames []string

	for _, export := range exportResponse["data"].([]interface{}) {
		exportNames = append(exportNames, export.(map[string]interface{})["name"].(string))
	}

	return exportNames
}
