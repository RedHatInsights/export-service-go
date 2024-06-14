package s3

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	econfig "github.com/redhatinsights/export-service-go/config"
	"go.uber.org/zap"

	export_logger "github.com/redhatinsights/export-service-go/logger"
	"github.com/redhatinsights/export-service-go/models"
)

const formatDateTime = "2006-01-02T15:04:05Z" // ISO 8601

type Compressor struct {
	Bucket string
	Log    *zap.SugaredLogger
	Client s3.Client
	Cfg    econfig.ExportConfig
}

// S3ListObjectsAPI defines the interface for the ListObjectsV2 function.
// We use this interface to test the function using a mocked service.
// https://aws.github.io/aws-sdk-go-v2/docs/code-examples/s3/listobjects/
type S3ListObjectsAPI interface {
	ListObjectsV2(ctx context.Context,
		params *s3.ListObjectsV2Input,
		optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
}

type StorageHandler interface {
	Compress(ctx context.Context, m *models.ExportPayload) (time.Time, string, string, error)
	Download(ctx context.Context, w io.WriterAt, bucket, key *string) (n int64, err error)
	Upload(ctx context.Context, body io.Reader, bucket, key *string) (*manager.UploadOutput, error)
	CreateObject(ctx context.Context, db models.DBInterface, body io.Reader, application string, resourceUUID uuid.UUID, payload *models.ExportPayload) error
	GetObject(ctx context.Context, key string) (io.ReadCloser, error)
	ProcessSources(db models.DBInterface, uid uuid.UUID)
}

func GetObjects(c context.Context, api S3ListObjectsAPI, input *s3.ListObjectsV2Input) (*s3.ListObjectsV2Output, error) {
	return api.ListObjectsV2(c, input)
}

func (c *Compressor) zipExport(ctx context.Context, prefix, filename, s3key string, meta ExportMeta, sources []models.Source) error {
	input := &s3.ListObjectsV2Input{
		Bucket: &c.Bucket,
		Prefix: &prefix,
	}

	var fileMeta []ExportFileMeta

	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	s3client := NewS3Client(c.Cfg, c.Log)

	resp, err := GetObjects(ctx, s3client, input)
	if err != nil {
		return fmt.Errorf("failed to list bucket objects: %w", err)
	}

	for _, obj := range resp.Contents {

		c.Log.Infof("downloading s3://%s/%s...", c.Bucket, *obj.Key)
		basename := filepath.Base(*obj.Key)

		// save id from the basename without the extension
		id := strings.Split(basename, ".")[0]

		f, err := os.CreateTemp("", basename)
		if err != nil {
			return fmt.Errorf("failed to create temp file: %w", err)
		}
		if _, err := c.Download(ctx, f, &c.Bucket, obj.Key); err != nil {
			return fmt.Errorf("failed to download to file: %w", err)
		}
		fi, err := f.Stat()
		if err != nil {
			return fmt.Errorf("failed to get file info: %w", err)
		}

		tempFileMeta, err := findFileMeta(id, basename, sources)

		if err != nil {
			return fmt.Errorf("failed to parse file meta: %w", err)
		}

		fileMeta = append(fileMeta, *tempFileMeta)

		header, err := zip.FileInfoHeader(fi)
		if err != nil {
			return fmt.Errorf("failed to create file header: %w", err)
		}
		header.Name = basename

		var zippedFile io.Writer
		if zippedFile, err = zipWriter.CreateHeader(header); err != nil {
			return fmt.Errorf("failed to write header: %w", err)
		}
		if _, err := io.Copy(zippedFile, f); err != nil {
			return fmt.Errorf("failed to copy data into tar file: %w", err)
		}
		c.Log.Infof("added file %s to payload", basename)
	}

	// add the file metadata to the ExportMeta struct
	meta.FileMeta = fileMeta

	metaJSON, err := BuildMeta(&meta)
	if err != nil {
		return fmt.Errorf("failed to marshal meta struct: %w", err)
	}

	readme, err := BuildReadme(&meta)
	if err != nil {
		return fmt.Errorf("failed to build README.md: %w", err)
	}

	var files = []struct {
		Name string
		Body []byte
	}{
		{"meta.json", metaJSON},
		{"README.md", []byte(readme)},
	}

	for _, fileToAdd := range files {
		zipFile, err := zipWriter.Create(fileToAdd.Name)
		if err != nil {
			return fmt.Errorf("failed to create file %s in zip file: %w", err)
		}

		_, err = zipFile.Write(fileToAdd.Body)
		if err != nil {
			return fmt.Errorf("failed to write file %s to zip file: %w", err)
		}
	}

	// produce zip
	if err := zipWriter.Close(); err != nil {
		return fmt.Errorf("failed to close gzip writer: %w", err)
	}

	f, err := os.CreateTemp("", filename)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	if _, err := io.Copy(f, &buf); err != nil {
		return fmt.Errorf("failed to copy buffer into file: %w", err)
	}

	c.Log.Infof("saving temp file %s", filename)
	c.Log.Infof("shipping %s to s3", filename)

	// seek to the beginning of the file so that we can reuse the file handler for upload
	if _, err := f.Seek(0, 0); err != nil {
		return fmt.Errorf("failed to seek to beginning of file: %w", err)
	}

	if _, err := c.Upload(ctx, f, &c.Cfg.StorageConfig.Bucket, &s3key); err != nil {
		return fmt.Errorf("failed to upload tarfile `%s` to s3: %w", s3key, err)
	}

	return nil
}

func (c *Compressor) Compress(ctx context.Context, m *models.ExportPayload) (time.Time, string, string, error) {
	t := time.Now()

	c.Log.Infof("starting payload compression for %s", m.ID)
	prefix := fmt.Sprintf("%s/%s/", m.OrganizationID, m.ID)
	filename := fmt.Sprintf("%s-%s.tar.gz", t.UTC().Format(formatDateTime), m.ID.String())
	s3key := fmt.Sprintf("%s/%s", m.OrganizationID, filename)

	sources, err := m.GetSources()
	if err != nil {
		return t, filename, s3key, fmt.Errorf("failed to get sources: %w", err)
	}

	meta := ExportMeta{
		ExportBy:    m.User.Username,
		ExportDate:  m.CreatedAt.UTC().Format(formatDateTime),
		ExportOrgID: m.User.OrganizationID,
		HelpString:  helpString,
	}

	err = c.zipExport(ctx, prefix, filename, s3key, meta, sources)
	return t, filename, s3key, err
}

func (c *Compressor) Download(ctx context.Context, w io.WriterAt, bucket, key *string) (n int64, err error) {
	s3client := NewS3Client(c.Cfg, c.Log)

	downloader := manager.NewDownloader(s3client, func(d *manager.Downloader) {
		d.PartSize = 100 * 1024 * 1024 // 100 MiB
	})

	input := &s3.GetObjectInput{Bucket: bucket, Key: key}

	return downloader.Download(ctx, w, input)
}

func (c *Compressor) Upload(ctx context.Context, body io.Reader, bucket, key *string) (*manager.UploadOutput, error) {
	s3client := NewS3Client(c.Cfg, c.Log)

	uploader := manager.NewUploader(s3client, func(u *manager.Uploader) {
		u.PartSize = 100 * 1024 * 1024 // 100 MiB
	})

	input := &s3.PutObjectInput{
		Bucket: bucket,
		Key:    key,
		Body:   body,
	}

	result, err := uploader.Upload(ctx, input)
	if err != nil {
		c.Log.Errorf("failed to uplodad tarfile `%s` to s3: %v", *key, err)

		deleteInput := &s3.DeleteObjectInput{
			Bucket: bucket,
			Key:    key,
		}

		_, deleteErr := s3client.DeleteObject(ctx, deleteInput)
		if deleteErr != nil {
			c.Log.Errorf("failed to delete partially upload object `%s` from s3: %v", *key, deleteErr)
		}
		return nil, err
	}
	return result, nil
}

func getUploadSize(ctx context.Context, s3client *s3.Client, bucket, key *string) (int64, error) {
	headObj := &s3.HeadObjectInput{
		Bucket: bucket,
		Key:    key,
	}
	headObjOutput, err := s3client.HeadObject(ctx, headObj)
	if err != nil {
		return 0, err
	}
	return headObjOutput.ContentLength, nil
}

func (c *Compressor) CreateObject(ctx context.Context, db models.DBInterface, body io.Reader, application string, resourceUUID uuid.UUID, payload *models.ExportPayload) error {
	filename := fmt.Sprintf("%s/%s/%s.%s", payload.OrganizationID, payload.ID, resourceUUID, payload.Format)

	if err := payload.SetStatusRunning(db); err != nil {
		c.Log.Errorw("failed to set running status", "error", err)
		return err
	}

	_, uploadErr := c.Upload(ctx, body, &c.Bucket, &filename)
	totalUploads.Inc()
	if uploadErr != nil {
		failUploads.Inc()
		c.Log.Errorf("error during upload: %v", uploadErr)
		statusError := models.SourceError{Message: uploadErr.Error(), Code: 1} // TODO: determine a better approach to assigning an internal status code
		if err := payload.SetSourceStatus(db, resourceUUID, models.RFailed, &statusError); err != nil {
			c.Log.Errorw("failed to set source status after failed upload", "error", err)
			return uploadErr
		}
		return uploadErr
	}

	uploadSize, err := getUploadSize(ctx, &c.Client, &c.Bucket, &filename)
	if err != nil {
		c.Log.Errorw("failed to get metric for upload size", "error", err)
	} else {
		uploadSizes.With(prometheus.Labels{"account": payload.AccountID, "org_id": payload.OrganizationID, "app": application}).Observe(float64(uploadSize))
	}

	return nil
}

func (c *Compressor) GetObject(ctx context.Context, key string) (io.ReadCloser, error) {
	input := &s3.GetObjectInput{Bucket: &c.Bucket, Key: &key}
	//return GetObject(ctx, &c.Client, input)
	s3Object, err := GetObject(ctx, &c.Client, input)
	if err != nil {
		return nil, err
	}
	return s3Object.Body, err
}

func (c *Compressor) compressPayload(db models.DBInterface, payload *models.ExportPayload) {
	t, filename, s3key, err := c.Compress(context.TODO(), payload)
	if err != nil {
		c.Log.Errorw("failed to compress payload", "error", err)
		if err := payload.SetStatusFailed(db); err != nil {
			c.Log.Errorw("failed to set status failed", "error", err)
			return
		}
	}

	c.Log.Infof("done uploading %s", filename)
	ready, err := payload.GetAllSourcesStatus()
	if err != nil {
		c.Log.Errorf("failed to get all source status: %v", err)
		return
	}

	switch ready {
	case models.StatusComplete:
		err = payload.SetStatusComplete(db, &t, s3key)
	case models.StatusPartial:
		err = payload.SetStatusPartial(db, &t, s3key)
	}

	if err != nil {
		c.Log.Errorw("failed updating model status", "error", err)
		return
	}
}

func (c *Compressor) ProcessSources(db models.DBInterface, uid uuid.UUID) {

	logger := c.Log.With(export_logger.ExportIDField(uid.String()))

	payload, err := db.Get(uid)
	if err != nil {
		logger.Errorf("failed to get payload: %v", err)
		return
	}
	ready, err := payload.GetAllSourcesStatus()
	if err != nil {
		logger.Errorf("failed to get all source status: %v", err)
		return
	}
	switch ready {
	case models.StatusComplete, models.StatusPartial:
		if payload.Status == models.Running {
			logger.Infow("ready for zipping", "export-uuid", payload.ID)
			go c.compressPayload(db, payload) // start a go-routine to not block
		}
	case models.StatusPending:
		return
	case models.StatusFailed:
		logger.Infof("all sources for payload %s reported as failure", payload.ID)
		if err := payload.SetStatusFailed(db); err != nil {
			logger.Errorw("failed updating model status after sources failed", "error", err)
		}
	}
}

type MockStorageHandler struct {
}

func (mc *MockStorageHandler) Compress(ctx context.Context, m *models.ExportPayload) (time.Time, string, string, error) {
	fmt.Println("Ran mockStorageHandler.Compress")
	return time.Now(), "filename", "s3key", nil
}

func (mc *MockStorageHandler) Download(ctx context.Context, w io.WriterAt, bucket, key *string) (n int64, err error) {
	fmt.Println("Ran mockStorageHandler.Download")
	return 0, nil
}

func (mc *MockStorageHandler) Upload(ctx context.Context, body io.Reader, bucket, key *string) (*manager.UploadOutput, error) {
	fmt.Println("Ran mockStorageHandler.Upload")
	return nil, nil
}

func (mc *MockStorageHandler) CreateObject(ctx context.Context, db models.DBInterface, body io.Reader, application string, resourceUUID uuid.UUID, payload *models.ExportPayload) error {
	fmt.Println("Ran mockStorageHandler.CreateObject")
	return nil
}

func (mc *MockStorageHandler) GetObject(ctx context.Context, key string) (io.ReadCloser, error) {
	fmt.Println("Ran mockStorageHandler.GetObject")

	return nil, nil
}

func (mc *MockStorageHandler) ProcessSources(db models.DBInterface, uid uuid.UUID) {
	// set status to complete
	payload, err := db.Get(uid)
	if err != nil {
		fmt.Printf("failed to get payload: %v", err)
		return
	}
	ready, err := payload.GetAllSourcesStatus()
	if err != nil {
		fmt.Printf("failed to get all source status: %v", err)
		return
	}
	switch ready {
	case models.StatusComplete, models.StatusPartial:
		if err := payload.SetStatusComplete(db, nil, ""); err != nil {
			fmt.Printf("failed updating model status: %v", err)
			return
		}
	case models.StatusPending:
		return
	case models.StatusFailed:
		fmt.Printf("all sources for payload %s reported as failure", payload.ID)
		if err := payload.SetStatusFailed(db); err != nil {
			fmt.Printf("failed updating model status after sources failed: %v", err)
			return
		}
	}

	fmt.Println("Ran mockStorageHandler.ProcessSources")
}
