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
	Bucket     string
	Log        *zap.SugaredLogger
	Client     s3.Client
	Cfg        econfig.ExportConfig
	Uploader   *manager.Uploader
	Downloader *manager.Downloader
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
	Compress(ctx context.Context, logger *zap.SugaredLogger, m *models.ExportPayload) (time.Time, string, string, error)
	Download(ctx context.Context, logger *zap.SugaredLogger, w io.WriterAt, bucket, key *string) (n int64, err error)
	Upload(ctx context.Context, logger *zap.SugaredLogger, body io.Reader, bucket, key *string) (*manager.UploadOutput, error)
	CreateObject(ctx context.Context, logger *zap.SugaredLogger, db models.DBInterface, body io.Reader, application string, resourceUUID uuid.UUID, payload *models.ExportPayload) error
	GetObject(ctx context.Context, logger *zap.SugaredLogger, key string) (io.ReadCloser, error)
	ProcessSources(db models.DBInterface, uid uuid.UUID)
}

func GetObjects(c context.Context, api S3ListObjectsAPI, input *s3.ListObjectsV2Input) (*s3.ListObjectsV2Output, error) {
	return api.ListObjectsV2(c, input)
}

func (c *Compressor) zipExport(ctx context.Context, logger *zap.SugaredLogger, prefix, filename, s3key string, meta ExportMeta, sources []models.Source) error {
	// Use this temp directory for all temp files
	tempDirName, err := os.MkdirTemp("", filename)
	if err != nil {
		return err
	}

	// Delete the contents of the temp directory when this function returns
	defer os.RemoveAll(tempDirName)

	downloadedFiles, err := downloadFilesFromS3(ctx, c.Cfg, logger, c.Downloader, c.Bucket, prefix, tempDirName)
	if err != nil {
		return err
	}

	fileMetadata, err := buildFileMetadata(downloadedFiles, sources)
	if err != nil {
		return err
	}

	meta.FileMeta = fileMetadata

	zippedBuffer, err := writeFilesToZip(logger, downloadedFiles, meta)
	if err != nil {
		return err
	}

	tempExportFile, err := writeBufferToTempFile(logger, zippedBuffer, filename, tempDirName)
	if err != nil {
		return err
	}

	logger.Infof("shipping %s to s3", filename)
	if _, err := c.Upload(ctx, logger, tempExportFile, &c.Cfg.StorageConfig.Bucket, &s3key); err != nil {
		return fmt.Errorf("failed to upload zip file `%s` to s3: %w", s3key, err)
	}

	return nil
}

type s3FileData struct {
	file     *os.File
	basename string
}

func downloadFilesFromS3(ctx context.Context, cfg econfig.ExportConfig, log *zap.SugaredLogger, downloader *manager.Downloader, bucket string, prefix string, tempDir string) ([]s3FileData, error) {
	input := &s3.ListObjectsV2Input{
		Bucket: &bucket,
		Prefix: &prefix,
	}

	s3client := NewS3Client(cfg, log)

	resp, err := GetObjects(ctx, s3client, input)
	if err != nil {
		return nil, fmt.Errorf("failed to list bucket objects: %w", err)
	}

	if len(resp.Contents) < 1 {
		return nil, fmt.Errorf("failed to list bucket objects: %w", err)
	}

	downloadedFiles := make([]s3FileData, 0, len(resp.Contents))

	for _, obj := range resp.Contents {

		log.Infof("downloading s3://%s/%s...", bucket, *obj.Key)
		basename := filepath.Base(*obj.Key)

		f, err := os.CreateTemp(tempDir, basename)
		if err != nil {
			return nil, fmt.Errorf("failed to create temp file: %w", err)
		}

		input := &s3.GetObjectInput{Bucket: &bucket, Key: obj.Key}

		if _, err := downloader.Download(ctx, f, input); err != nil {
			return nil, fmt.Errorf("failed to download to file: %w", err)
		}

		downloadedFiles = append(downloadedFiles, s3FileData{f, basename})
	}

	return downloadedFiles, nil
}

func buildFileMetadata(files []s3FileData, sources []models.Source) ([]ExportFileMeta, error) {
	fileMeta := make([]ExportFileMeta, 0, len(files))

	for _, f := range files {

		id := strings.Split(f.basename, ".")[0]

		tempFileMeta, err := findFileMeta(id, f.basename, sources)
		if err != nil {
			return nil, err
		}

		fileMeta = append(fileMeta, *tempFileMeta)
	}

	return fileMeta, nil
}

func writeFilesToZip(log *zap.SugaredLogger, files []s3FileData, meta ExportMeta) (*bytes.Buffer, error) {
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	defer zipWriter.Close()

	for _, f := range files {

		fi, err := f.file.Stat()
		if err != nil {
			return nil, fmt.Errorf("failed to get file info: %w", err)
		}

		header, err := zip.FileInfoHeader(fi)
		if err != nil {
			return nil, fmt.Errorf("failed to create file header: %w", err)
		}
		header.Name = f.basename
		header.Method = zip.Deflate // DEFLATE compressed

		var zippedFile io.Writer
		if zippedFile, err = zipWriter.CreateHeader(header); err != nil {
			return nil, fmt.Errorf("failed to write header: %w", err)
		}

		if _, err := io.Copy(zippedFile, f.file); err != nil {
			return nil, fmt.Errorf("failed to copy data into zip file: %w", err)
		}

		log.Infof("added file %s to payload", f.basename)

	}

	if err := addMetadataFilesToZip(&meta, zipWriter); err != nil {
		return nil, err
	}

	return &buf, nil
}

func writeBufferToTempFile(log *zap.SugaredLogger, buf *bytes.Buffer, filename string, tempDir string) (*os.File, error) {
	f, err := os.CreateTemp(tempDir, filename)
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}

	if _, err := io.Copy(f, buf); err != nil {
		return nil, fmt.Errorf("failed to copy buffer into file: %w", err)
	}

	log.Infof("saving temp file %s", filename)

	// seek to the beginning of the file so that we can reuse the file handler for upload
	if _, err := f.Seek(0, 0); err != nil {
		return nil, fmt.Errorf("failed to seek to beginning of file: %w", err)
	}

	return f, nil
}

func addMetadataFilesToZip(meta *ExportMeta, zipWriter *zip.Writer) error {
	metaJSON, err := BuildMeta(meta)
	if err != nil {
		return fmt.Errorf("failed to marshal meta struct: %w", err)
	}

	readme, err := BuildReadme(meta)
	if err != nil {
		return fmt.Errorf("failed to build README.md: %w", err)
	}

	metadataFiles := []struct {
		Name string
		Body []byte
	}{
		{"meta.json", metaJSON},
		{"README.md", []byte(readme)},
	}

	for _, fileToAdd := range metadataFiles {
		zipFile, err := zipWriter.Create(fileToAdd.Name)
		if err != nil {
			return fmt.Errorf("failed to create file %s in zip file: %w", fileToAdd.Name, err)
		}

		_, err = zipFile.Write(fileToAdd.Body)
		if err != nil {
			return fmt.Errorf("failed to write file %s to zip file: %w", fileToAdd.Name, err)
		}
	}

	return nil
}

func (c *Compressor) Compress(ctx context.Context, logger *zap.SugaredLogger, m *models.ExportPayload) (time.Time, string, string, error) {
	t := time.Now()

	logger.Infof("starting payload compression for %s", m.ID)
	prefix := fmt.Sprintf("%s/%s/", m.OrganizationID, m.ID)
	filename := fmt.Sprintf("%s-%s.zip", t.UTC().Format(formatDateTime), m.ID.String())
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

	err = c.zipExport(ctx, logger, prefix, filename, s3key, meta, sources)
	return t, filename, s3key, err
}

func (c *Compressor) Download(ctx context.Context, logger *zap.SugaredLogger, w io.WriterAt, bucket, key *string) (n int64, err error) {
	input := &s3.GetObjectInput{Bucket: bucket, Key: key}

	return c.Downloader.Download(ctx, w, input)
}

func (c *Compressor) Upload(ctx context.Context, logger *zap.SugaredLogger, body io.Reader, bucket, key *string) (*manager.UploadOutput, error) {
	s3client := NewS3Client(c.Cfg, c.Log)

	input := &s3.PutObjectInput{
		Bucket: bucket,
		Key:    key,
		Body:   body,
	}

	result, err := c.Uploader.Upload(ctx, input)
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
	if err != nil || (headObjOutput == nil || headObjOutput.ContentLength == nil) {
		return 0, err
	}

	return *headObjOutput.ContentLength, nil
}

func (c *Compressor) CreateObject(ctx context.Context, logger *zap.SugaredLogger, db models.DBInterface, body io.Reader, application string, resourceUUID uuid.UUID, payload *models.ExportPayload) error {
	filename := fmt.Sprintf("%s/%s/%s.%s", payload.OrganizationID, payload.ID, resourceUUID, payload.Format)

	if err := payload.SetStatusRunning(db); err != nil {
		logger.Errorw("failed to set running status", "error", err)
		return err
	}

	_, uploadErr := c.Upload(ctx, logger, body, &c.Bucket, &filename)
	totalUploads.Inc()
	if uploadErr != nil {
		failUploads.Inc()
		logger.Errorf("error during upload: %v", uploadErr)
		statusError := models.SourceError{Message: uploadErr.Error(), Code: 1} // TODO: determine a better approach to assigning an internal status code
		if err := payload.SetSourceStatus(db, resourceUUID, models.RFailed, &statusError); err != nil {
			logger.Errorw("failed to set source status after failed upload", "error", err)
			return uploadErr
		}
		return uploadErr
	}

	uploadSize, err := getUploadSize(ctx, &c.Client, &c.Bucket, &filename)
	if err != nil {
		logger.Errorw("failed to get metric for upload size", "error", err)
	} else {
		uploadSizes.With(prometheus.Labels{"app": application}).Observe(float64(uploadSize))
	}

	return nil
}

func (c *Compressor) GetObject(ctx context.Context, logger *zap.SugaredLogger, key string) (io.ReadCloser, error) {
	input := &s3.GetObjectInput{Bucket: &c.Bucket, Key: &key}
	// return GetObject(ctx, &c.Client, input)
	s3Object, err := GetObject(ctx, &c.Client, input)
	if err != nil {
		return nil, err
	}
	return s3Object.Body, err
}

func (c *Compressor) compressPayload(logger *zap.SugaredLogger, db models.DBInterface, payload *models.ExportPayload) {
	t, filename, s3key, err := c.Compress(context.TODO(), logger, payload)
	if err != nil {
		logger.Errorw("failed to compress payload", "error", err)
		if err := payload.SetStatusFailed(db); err != nil {
			logger.Errorw("failed to set status failed", "error", err)
			return
		}
	}

	logger.Infof("done uploading %s", filename)
	ready, err := payload.GetAllSourcesStatus()
	if err != nil {
		logger.Errorf("failed to get all source status: %v", err)
		return
	}

	switch ready {
	case models.StatusComplete:
		err = payload.SetStatusComplete(db, &t, s3key)
	case models.StatusPartial:
		err = payload.SetStatusPartial(db, &t, s3key)
	}

	if err != nil {
		logger.Errorw("failed updating model status", "error", err)
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
			go c.compressPayload(logger, db, payload) // start a go-routine to not block
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

type MockStorageHandler struct{}

func (mc *MockStorageHandler) Compress(ctx context.Context, l *zap.SugaredLogger, m *models.ExportPayload) (time.Time, string, string, error) {
	fmt.Println("Ran mockStorageHandler.Compress")
	return time.Now(), "filename", "s3key", nil
}

func (mc *MockStorageHandler) Download(ctx context.Context, l *zap.SugaredLogger, w io.WriterAt, bucket, key *string) (n int64, err error) {
	fmt.Println("Ran mockStorageHandler.Download")
	return 0, nil
}

func (mc *MockStorageHandler) Upload(ctx context.Context, l *zap.SugaredLogger, body io.Reader, bucket, key *string) (*manager.UploadOutput, error) {
	fmt.Println("Ran mockStorageHandler.Upload")
	return nil, nil
}

func (mc *MockStorageHandler) CreateObject(ctx context.Context, l *zap.SugaredLogger, db models.DBInterface, body io.Reader, application string, resourceUUID uuid.UUID, payload *models.ExportPayload) error {
	fmt.Println("Ran mockStorageHandler.CreateObject")
	return nil
}

func (mc *MockStorageHandler) GetObject(ctx context.Context, l *zap.SugaredLogger, key string) (io.ReadCloser, error) {
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
