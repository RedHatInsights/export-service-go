package models_test

import (
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	m "github.com/redhatinsights/export-service-go/models"
)

var _ = Describe("Db", func() {
	var exportPayload *m.ExportPayload

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

	Describe("Create", func() {
		Context("when the create operation succeeds", func() {
			It("should return the payload and no error", func() {
				setupTest(testGormDB)
				result, err := exportDB.Create(exportPayload)
				Expect(result).To(Equal(exportPayload))
				Expect(err).To(BeNil())
			})
		})
	})

	Describe("Delete", func() {
		Context("when the delete operation succeeds", func() {
			It("should return no error", func() {
				setupTest(testGormDB)
				user := m.User{AccountID: "1234", OrganizationID: "5678", Username: "batman"}
				exportPayload.User = user

				result, err := exportDB.Create(exportPayload)
				Expect(result).To(Equal(exportPayload))
				Expect(err).To(BeNil())

				deleteErr := exportDB.Delete(exportPayload.ID, user)
				Expect(deleteErr).To(BeNil())
			})
		})
	})

	Describe("Get", func() {
		Context("when the get operation succeeds", func() {
			It("should return the payload and no error", func() {
				setupTest(testGormDB)
				user := m.User{AccountID: "1234", OrganizationID: "5678", Username: "batman"}
				exportPayload.User = user

				result, err := exportDB.Create(exportPayload)
				Expect(result).To(Equal(exportPayload))
				Expect(err).To(BeNil())

				getResult, getErr := exportDB.Get(exportPayload.ID)
				Expect(getErr).To(BeNil())
				Expect(getResult.ID).To(Equal(exportPayload.ID))
				Expect(getResult.RequestID).To(Equal(exportPayload.RequestID))
				Expect(getResult.Name).To(Equal(exportPayload.Name))
				Expect(getResult.Status).To(Equal(exportPayload.Status))
				Expect(getResult.User).To(Equal(exportPayload.User))
			})
		})
	})

	Describe("GetWithUser", func() {
		Context("when the get operation succeeds", func() {
			It("should return the payload and no error", func() {
				setupTest(testGormDB)
				user := m.User{AccountID: "1234", OrganizationID: "5678", Username: "batman"}
				exportPayload.User = user

				result, err := exportDB.Create(exportPayload)
				Expect(result).To(Equal(exportPayload))
				Expect(err).To(BeNil())

				getResult, getErr := exportDB.GetWithUser(exportPayload.ID, user)
				Expect(getErr).To(BeNil())
				Expect(getResult.ID).To(Equal(exportPayload.ID))
				Expect(getResult.User).To(Equal(exportPayload.User))
			})
		})
	})

	Describe("List", func() {
		Context("when the list operation succeeds", func() {
			It("should return the exports for the user and no error", func() {
				setupTest(testGormDB)
				user := m.User{AccountID: "1234", OrganizationID: "5678", Username: "batman"}
				exportPayload.User = user

				result, err := exportDB.Create(exportPayload)
				Expect(result).To(Equal(exportPayload))
				Expect(err).To(BeNil())

				listResult, listErr := exportDB.List(user)
				Expect(listErr).To(BeNil())
				Expect(listResult[0].ID).To(Equal(exportPayload.ID))
				Expect(listResult[0].RequestID).To(Equal(exportPayload.RequestID))
				Expect(listResult[0].Name).To(Equal(exportPayload.Name))
				Expect(listResult[0].Status).To(Equal(exportPayload.Status))
				Expect(listResult[0].User).To(Equal(exportPayload.User))
			})
		})
	})

	Describe("Updates", func() {
		Context("when the update operation succeeds", func() {
			It("should return no errors", func() {
				setupTest(testGormDB)
				user := m.User{AccountID: "1234", OrganizationID: "5678", Username: "batman"}
				exportPayload.User = user

				result, err := exportDB.Create(exportPayload)
				Expect(result).To(Equal(exportPayload))
				Expect(err).To(BeNil())

				values := map[string]interface{}{"name": "new-export-name"}
				updateErr := exportDB.Updates(exportPayload, values)
				Expect(updateErr).To(BeNil())

				getResult, getErr := exportDB.Get(exportPayload.ID)
				Expect(getErr).To(BeNil())
				Expect(getResult.Name).To(Equal("new-export-name"))
			})
		})
	})

	Describe("DeleteExpiredExports", func() {
		Context("when the DeleteExpiredExports operation succeeds", func() {
			It("should delete all expired exports and return no errors", func() {
				setupTest(testGormDB)

				user := m.User{AccountID: "1234", OrganizationID: "5678", Username: "batman"}
				exportPayload.User = user

				currentTime := time.Now()
				eightDaysAgo := currentTime.Add(-8 * 24 * time.Hour)
				exportPayload.CreatedAt = currentTime.Add(-9 * 24 * time.Hour)
				exportPayload.UpdatedAt = currentTime.Add(-9 * 24 * time.Hour)
				exportPayload.Expires = &eightDaysAgo

				result, err := exportDB.Create(exportPayload)
				Expect(result).To(Equal(exportPayload))
				Expect(err).To(BeNil())

				exportDeleteErr := exportDB.DeleteExpiredExports()
				Expect(exportDeleteErr).To(BeNil())

				_, getErr := exportDB.Get(exportPayload.ID)
				Expect(getErr).To(HaveOccurred())
				Expect(getErr.Error()).To(Equal("record not found"))
			})
		})
	})
})
