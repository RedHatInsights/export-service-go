package models_test

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	m "github.com/redhatinsights/export-service-go/models"
)


var _ = Describe("Models", func() {
	var (
		exportPayload *m.ExportPayload
	)

	BeforeEach(func() {
		exportPayload = &m.ExportPayload{
			ID:          uuid.New(),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
			CompletedAt: nil,
			Expires:     nil,
			RequestID:   "request-123",
			Name:        "payload",
			Format:      m.CSV,
			Status:      m.Pending,
			Sources:     []m.Source{},
			S3Key:       "s3-key",
			User:        m.User{},
		}
	})

	Describe("GetSource", func() {
		Context("when the source is found", func() {
			It("should return the index and source", func() {
				uid := uuid.New()
				source := m.Source{ID: uid}
				exportPayload.Sources = append(exportPayload.Sources, source)

				index, resultSource, err := exportPayload.GetSource(uid)
				Expect(err).To(BeNil())
				Expect(index).To(Equal(0))
				Expect(resultSource).To(Equal(&source))
			})
		})

		Context("when the source is not found", func() {
			It("should return an error", func() {
				uid := uuid.New()
				index, resultSource, err := exportPayload.GetSource(uid)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal(fmt.Sprintf("source `%s` not found", uid)))
				Expect(index).To(Equal(-1))
				Expect(resultSource).To(BeNil())
			})
		})
	})

	Describe("GetSources", func() {
		It("should return the list of sources", func() {
			uid1 := uuid.New()
			uid2 := uuid.New()
			exportPayload.Sources = append(exportPayload.Sources, m.Source{ID: uid1}, m.Source{ID: uid2})

			sources, err := exportPayload.GetSources()
			Expect(err).To(BeNil())
			Expect(sources).To(Equal(exportPayload.Sources))
		})
	})

	Describe("SetStatusComplete", func() {
		It("should set the status complete for payloads", func() {
			setupTest(testGormDB)

			createdExport, err := exportDB.Create(exportPayload)
			Expect(err).To(BeNil())

			payloadId := createdExport.ID
			completionTime := time.Now()
			s3Key := "test"

			statusUpdateErr := exportPayload.SetStatusComplete(exportDB, &completionTime, s3Key)
			Expect(statusUpdateErr).To(BeNil())

			result, err := exportDB.Get(payloadId)
			Expect(err).To(BeNil())
			Expect(result.Status).To(Equal(m.Complete))
			Expect(result.S3Key).To(Equal(s3Key))

			expectedCompletionTime := *result.CompletedAt
			Expect(expectedCompletionTime.Truncate(time.Millisecond)).Should(Equal(completionTime.Truncate(time.Millisecond)))
		})
	})

	Describe("SetStatusPartial", func() {
		It("should set the status partial for payloads", func() {
			setupTest(testGormDB)

			createdExport, err := exportDB.Create(exportPayload)
			Expect(err).To(BeNil())

			payloadId := createdExport.ID
			completionTime := time.Now()
			s3Key := "test"

			statusUpdateErr := exportPayload.SetStatusPartial(exportDB, &completionTime, s3Key)
			Expect(statusUpdateErr).To(BeNil())

			result, err := exportDB.Get(payloadId)
			Expect(err).To(BeNil())
			Expect(result.Status).To(Equal(m.Partial))
			Expect(result.S3Key).To(Equal(s3Key))

			expectedCompletionTime := *result.CompletedAt
			Expect(expectedCompletionTime.Truncate(time.Millisecond)).Should(Equal(completionTime.Truncate(time.Millisecond)))
		})
	})

	Describe("SetStatusFailed", func() {
		It("should set the status failed for payloads", func() {
			setupTest(testGormDB)

			createdExport, err := exportDB.Create(exportPayload)
			Expect(err).To(BeNil())

			payloadId := createdExport.ID

			statusUpdateErr := exportPayload.SetStatusFailed(exportDB)
			Expect(statusUpdateErr).To(BeNil())

			result, err := exportDB.Get(payloadId)
			Expect(err).To(BeNil())
			Expect(result.Status).To(Equal(m.Failed))
		})
	})

	Describe("SetStatusRunning", func() {
		It("should set the status running for payloads", func() {
			setupTest(testGormDB)

			createdExport, err := exportDB.Create(exportPayload)
			Expect(err).To(BeNil())

			payloadId := createdExport.ID

			statusUpdateErr := exportPayload.SetStatusRunning(exportDB)
			Expect(statusUpdateErr).To(BeNil())

			result, err := exportDB.Get(payloadId)
			Expect(err).To(BeNil())
			Expect(result.Status).To(Equal(m.Running))
		})
	})

	Describe("SetSourceStatus", func() {
		It("should set a status for source", func() {
			setupTest(testGormDB)

			source := m.Source{Application: "test-app", Status: m.RPending}
			exportPayload.Sources = append(exportPayload.Sources, source)

			createdExport, err := exportDB.Create(exportPayload)
			Expect(err).To(BeNil())

			createdSource := createdExport.Sources[0]
			createdSourceID := createdSource.ID

			sourceStatusUpdateErr := exportPayload.SetSourceStatus(exportDB, createdSourceID, m.RSuccess, nil)
			Expect(sourceStatusUpdateErr).To(BeNil())

			updatedSource := m.Source{}
			result := testGormDB.First(&updatedSource, createdSourceID)
			Expect(result.Error).To(BeNil())

			Expect(updatedSource.Status).To(Equal(m.RSuccess))
		})

		It("should set a status for source and include source error", func() {
			setupTest(testGormDB)

			source := m.Source{Application: "test-app", Status: m.RPending}
			exportPayload.Sources = append(exportPayload.Sources, source)

			createdExport, err := exportDB.Create(exportPayload)
			Expect(err).To(BeNil())

			createdSource := createdExport.Sources[0]
			createdSourceID := createdSource.ID
			sourceError := &m.SourceError{Message: "sourceError", Code: 400}

			sourceStatusUpdateErr := exportPayload.SetSourceStatus(exportDB, createdSourceID, m.RFailed, sourceError)
			Expect(sourceStatusUpdateErr).To(BeNil())

			updatedSource := m.Source{}
			result := testGormDB.First(&updatedSource, createdSourceID)
			Expect(result.Error).To(BeNil())

			Expect(updatedSource.Status).To(Equal(m.RFailed))
			Expect(updatedSource.SourceError.Message).To(Equal(sourceError.Message))
			Expect(updatedSource.SourceError.Code).To(Equal(sourceError.Code))
		})
	})

	DescribeTable("GetAllSourcesStatus",
		func(sources []m.Source, expectedStatus int, expectedErr error) {
			exportPayload.Sources = sources

			status, err := exportPayload.GetAllSourcesStatus()

			if expectedErr == nil {
				Expect(err).To(BeNil())
			} else {
				Expect(err).To(Equal(expectedErr))
			}
			Expect(status).To(Equal(expectedStatus))
		},
		Entry("should return StatusComplete when sources are all complete as success",
			[]m.Source{
				{Status: m.RSuccess},
				{Status: m.RSuccess},
				{Status: m.RSuccess},
			},
			m.StatusComplete,
			nil,
		),
		Entry("should return StatusPending when sources are still pending",
			[]m.Source{
				{Status: m.RPending},
				{Status: m.RSuccess},
				{Status: m.RPending},
			},
			m.StatusPending,
			nil,
		),
		Entry("should return StatusFailed when all sources have failed",
			[]m.Source{
				{Status: m.RFailed},
				{Status: m.RFailed},
				{Status: m.RFailed},
			},
			m.StatusFailed,
			nil,
		),
		Entry("should return StatusPartial when sources are all complete, some sources are a failure",
			[]m.Source{
				{Status: m.RSuccess},
				{Status: m.RSuccess},
				{Status: m.RFailed},
			},
			m.StatusPartial,
			nil,
		),
	)
})
