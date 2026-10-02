package s3

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/redhatinsights/export-service-go/models"
)

// mockDB implements models.DBInterface for testing. Only Updates is exercised;
// it records the values passed so tests can inspect the resulting status.
type mockDB struct {
	updatedStatus models.PayloadStatus
	updatesCalled bool
	updatesErr    error
}

func (m *mockDB) Updates(ep *models.ExportPayload, values interface{}) error {
	m.updatesCalled = true
	if v, ok := values.(models.ExportPayload); ok {
		m.updatedStatus = v.Status
	}
	return m.updatesErr
}

// Unused DBInterface methods — panics guard against unexpected calls.
func (m *mockDB) APIList(_ models.User, _ *models.QueryParams, _, _ int, _, _ string) ([]*models.APIExport, int64, error) {
	panic("not implemented")
}
func (m *mockDB) Create(_ *models.ExportPayload) (*models.ExportPayload, error) {
	panic("not implemented")
}
func (m *mockDB) Delete(_ uuid.UUID, _ models.User) error  { panic("not implemented") }
func (m *mockDB) Get(_ uuid.UUID) (*models.ExportPayload, error) { panic("not implemented") }
func (m *mockDB) GetWithUser(_ uuid.UUID, _ models.User) (*models.ExportPayload, error) {
	panic("not implemented")
}
func (m *mockDB) List(_ models.User) ([]*models.ExportPayload, error) { panic("not implemented") }
func (m *mockDB) Raw(_ string, _ ...interface{}) *gorm.DB             { panic("not implemented") }
func (m *mockDB) DeleteExpiredExports() error                         { panic("not implemented") }

func newTestLogger() *zap.SugaredLogger {
	logger, _ := zap.NewDevelopment()
	return logger.Sugar()
}

func newTestPayload(sourceStatuses ...models.ResourceStatus) *models.ExportPayload {
	sources := make([]models.Source, len(sourceStatuses))
	for i, s := range sourceStatuses {
		sources[i] = models.Source{
			ID:     uuid.New(),
			Status: s,
		}
	}
	return &models.ExportPayload{
		ID:     uuid.New(),
		User:   models.User{OrganizationID: "test-org", Username: "test-user"},
		Status: models.Running,
		Sources: sources,
		CreatedAt: time.Now(),
	}
}

// TestCompressAndSetStatus_CompressError verifies that when Compress returns an
// error (including context.DeadlineExceeded), the payload status is set to
// Failed and never overwritten with Complete or Partial.
func TestCompressAndSetStatus_CompressError(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "deadline exceeded sets status to Failed",
			err:  context.DeadlineExceeded,
		},
		{
			name: "wrapped deadline exceeded sets status to Failed",
			err:  fmt.Errorf("compress failed: %w", context.DeadlineExceeded),
		},
		{
			name: "generic error sets status to Failed",
			err:  fmt.Errorf("unexpected S3 error"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := &mockDB{}
			logger := newTestLogger()
			payload := newTestPayload(models.RComplete)

			// Mock compress function that always returns the given error.
			mockCompress := func(_ context.Context, _ *zap.SugaredLogger, _ *models.ExportPayload) (time.Time, string, string, error) {
				return time.Time{}, "", "", tc.err
			}

			compressAndSetStatus(mockCompress, 5*time.Second, logger, db, payload)

			if !db.updatesCalled {
				t.Fatal("expected Updates to be called to set status Failed")
			}
			if db.updatedStatus != models.Failed {
				t.Errorf("expected status %q, got %q", models.Failed, db.updatedStatus)
			}
		})
	}
}

// TestCompressAndSetStatus_Success verifies that when Compress succeeds and all
// sources are complete, the payload status is set to Complete.
func TestCompressAndSetStatus_Success(t *testing.T) {
	db := &mockDB{}
	logger := newTestLogger()
	payload := newTestPayload(models.RComplete, models.RComplete)

	mockCompress := func(_ context.Context, _ *zap.SugaredLogger, _ *models.ExportPayload) (time.Time, string, string, error) {
		return time.Now(), "test.zip", "org/test.zip", nil
	}

	compressAndSetStatus(mockCompress, 5*time.Second, logger, db, payload)

	if !db.updatesCalled {
		t.Fatal("expected Updates to be called to set status Complete")
	}
	if db.updatedStatus != models.Complete {
		t.Errorf("expected status %q, got %q", models.Complete, db.updatedStatus)
	}
}

// TestCompressAndSetStatus_PartialSources verifies that when Compress succeeds
// but some sources failed, the payload status is set to Partial.
func TestCompressAndSetStatus_PartialSources(t *testing.T) {
	db := &mockDB{}
	logger := newTestLogger()
	payload := newTestPayload(models.RComplete, models.RFailed)

	mockCompress := func(_ context.Context, _ *zap.SugaredLogger, _ *models.ExportPayload) (time.Time, string, string, error) {
		return time.Now(), "test.zip", "org/test.zip", nil
	}

	compressAndSetStatus(mockCompress, 5*time.Second, logger, db, payload)

	if !db.updatesCalled {
		t.Fatal("expected Updates to be called to set status Partial")
	}
	if db.updatedStatus != models.Partial {
		t.Errorf("expected status %q, got %q", models.Partial, db.updatedStatus)
	}
}
