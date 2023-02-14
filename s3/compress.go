package s3

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/redhatinsights/export-service-go/models"
)

type Compressor struct {
	Bucket string
	Log    *zap.SugaredLogger
	Client s3.Client
}

// S3ListObjectsAPI defines the interface for the ListObjectsV2 function.
// We use this interface to test the function using a mocked service.
// https://aws.github.io/aws-sdk-go-v2/docs/code-examples/s3/listobjects/
type S3ListObjectsAPI interface {
	ListObjectsV2(ctx context.Context,
		params *s3.ListObjectsV2Input,
		optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
}

// struct used to fill the README.md and meta.json files in the tar
type ExportMeta struct {
	ExportBy    string           `json:"exported_by"`
	ExportDate  string           `json:"export_date"`
	ExportOrgID string           `json:"export_org_id"`
	FileMeta    []ExportFileMeta `json:"file_meta"`
	HelpString  string           `json:"help_string"`
}

// details for each file in the tar
type ExportFileMeta struct {
	Filename    string `json:"filename"`
	Application string `json:"application"`
	Resource    string `json:"resource"`
	// Filters are a key-value pair of the filters used to create the export
	Filters map[string]string `json:"filters"`
}

const (
	helpString = `Contained in this archive are your requested resources. If you need help or have any questions, please contact our team on slack @crc-pipeline-team.`
)

type StorageHandler interface {
	Compress(ctx context.Context, m *models.ExportPayload) (time.Time, string, string, error)
	Download(ctx context.Context, w io.WriterAt, bucket, key *string) (n int64, err error)
	Upload(ctx context.Context, body io.Reader, bucket, key *string) (*manager.UploadOutput, error)
	CreateObject(ctx context.Context, db models.DBInterface, body io.Reader, urlparams *models.URLParams, payload *models.ExportPayload) error
	GetObject(ctx context.Context, key string) (io.ReadCloser, error)
	ProcessSources(db models.DBInterface, uid uuid.UUID)
}

func GetObjects(c context.Context, api S3ListObjectsAPI, input *s3.ListObjectsV2Input) (*s3.ListObjectsV2Output, error) {
	return api.ListObjectsV2(c, input)
}

func (c *Compressor) zipExport(ctx context.Context, t time.Time, prefix, filename, s3key string, m *models.ExportPayload) error {
	input := &s3.ListObjectsV2Input{
		Bucket: &c.Bucket,
		Prefix: &prefix,
	}

	var fileMeta []ExportFileMeta

	var buf bytes.Buffer
	gzipWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzipWriter)

	resp, err := GetObjects(ctx, Client, input)
	if err != nil {
		return fmt.Errorf("failed to list bucket objects: %w", err)
	}

	// parse the models.ExportPayload.Sources into the models.Source struct
	var sources []models.Source
	if err := json.Unmarshal([]byte(m.Sources), &sources); err != nil {
		return fmt.Errorf("failed to unmarshal sources: %w", err)
	}

	for _, obj := range resp.Contents {

		c.Log.Infof("downloading s3://%s/%s...", c.Bucket, *obj.Key)
		basename := filepath.Base(*obj.Key)

		// save var id from the basename without the extension
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

		for _, source := range sources {
			if source.ID.String() == id {
				// parse Filters json into a map[string]string
				var filters map[string]string
				if err := json.Unmarshal([]byte(source.Filters), &filters); err != nil {
					return fmt.Errorf("failed to unmarshal filters: %w", err)
				}

				// set the ExportFileMeta struct fields
				fileMeta = append(fileMeta, ExportFileMeta{
					Filename:    basename,
					Application: source.Application,
					Resource:    source.Resource,
					Filters:     filters,
				})

				break
			}
		}

		header, err := tar.FileInfoHeader(fi, basename)
		if err != nil {
			return fmt.Errorf("failed to create file header: %w", err)
		}
		header.Name = basename

		if err = tarWriter.WriteHeader(header); err != nil {
			return fmt.Errorf("failed to write header: %w", err)
		}
		if _, err := io.Copy(tarWriter, f); err != nil {
			return fmt.Errorf("failed to copy data into tar file: %w", err)
		}
		c.Log.Infof("added file %s to payload", basename)
	}

	// create ExportMeta struct
	meta := ExportMeta{
		ExportBy:    m.User.Username,
		ExportDate:  m.CreatedAt.Format(time.RFC3339),
		ExportOrgID: m.User.OrganizationID,
		FileMeta:    fileMeta,
		HelpString:  helpString,
	}

	// make a json file from the ExportMeta struct
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("failed to marshal meta struct: %w", err)
	}

	// add the json file to the tar
	metaHeader := &tar.Header{
		Name: "meta.json",
		Mode: 0600,
		Size: int64(len(metaJSON)),
	}

	if err := tarWriter.WriteHeader(metaHeader); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}
	if _, err := tarWriter.Write(metaJSON); err != nil {
		return fmt.Errorf("failed to write meta.json: %w", err)
	}

	dataDetails := ""
	for _, file := range fileMeta {
		filterDetails := ""
		for key, value := range file.Filters {
			filterDetails += fmt.Sprintf(
				`
  - %s: %s`,
				key, value)
		}

		if filterDetails == "" {
			filterDetails = "None"
		}
		dataDetails += fmt.Sprintf(`
### %s
- **Application**: %s
- **Resource**: %s
- **Filters**: %s
`, file.Filename, file.Application, file.Resource, filterDetails)
	}

	if dataDetails == "" {
		dataDetails = "No data was found."
	}
	// next, make a README.md file containing the ExportMeta data in a readable format
	readme := fmt.Sprintf(`# Export Manifest

## Exported Information
- **Exported by**: %s
- **Org ID**: %s
- **Export Date**: %s

## Data Details
This archive contains the following data:
%s
## Help and Support
This service is owned by the ConsoldeDot Pipeline team. If you have any questions, or need support with this service, please contact the team on slack @crc-pipeline-team.

You can also raise an issue on the [Export Service GitHub repo](https://github.com/RedHatInsights/export-service-go/).
`,
		meta.ExportBy,
		meta.ExportOrgID,
		meta.ExportDate,
		dataDetails,
	)

	// add the README.md file to the tar
	readmeHeader := &tar.Header{
		Name: "README.md",
		Mode: 0600,
		Size: int64(len(readme)),
	}

	if err := tarWriter.WriteHeader(readmeHeader); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}
	if _, err := tarWriter.Write([]byte(readme)); err != nil {
		return fmt.Errorf("failed to write README.md: %w", err)
	}

	// produce tar
	if err := tarWriter.Close(); err != nil {
		return fmt.Errorf("failed to close tar writer: %w", err)
	}
	// produce gzip
	if err := gzipWriter.Close(); err != nil {
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

	if _, err := c.Upload(ctx, f, &cfg.StorageConfig.Bucket, &s3key); err != nil {
		return fmt.Errorf("failed to upload tarfile `%s` to s3: %w", s3key, err)
	}

	return nil
}

func (c *Compressor) Compress(ctx context.Context, m *models.ExportPayload) (time.Time, string, string, error) {
	t := time.Now()

	c.Log.Infof("starting payload compression for %s", m.ID)
	prefix := fmt.Sprintf("%s/%s/", m.OrganizationID, m.ID)
	filename := fmt.Sprintf("%s-%s.tar.gz", t.Format(time.RFC3339), m.ID.String())
	s3key := fmt.Sprintf("%s/%s", m.OrganizationID, filename)

	err := c.zipExport(ctx, t, prefix, filename, s3key, m)
	return t, filename, s3key, err
}

func (c *Compressor) Download(ctx context.Context, w io.WriterAt, bucket, key *string) (n int64, err error) {
	downloader := manager.NewDownloader(Client, func(d *manager.Downloader) {
		d.PartSize = 100 * 1024 * 1024 // 100 MiB
	})

	input := &s3.GetObjectInput{Bucket: bucket, Key: key}

	return downloader.Download(ctx, w, input)
}

func (c *Compressor) Upload(ctx context.Context, body io.Reader, bucket, key *string) (*manager.UploadOutput, error) {
	uploader := manager.NewUploader(Client, func(u *manager.Uploader) {
		u.PartSize = 100 * 1024 * 1024 // 100 MiB
	})

	input := &s3.PutObjectInput{
		Bucket: bucket,
		Key:    key,
		Body:   body,
	}
	return uploader.Upload(ctx, input)
}

func (c *Compressor) CreateObject(ctx context.Context, db models.DBInterface, body io.Reader, urlparams *models.URLParams, payload *models.ExportPayload) error {
	filename := fmt.Sprintf("%s/%s/%s.%s", payload.OrganizationID, payload.ID, urlparams.ResourceUUID, payload.Format)

	if err := payload.SetStatusRunning(db); err != nil {
		c.Log.Errorw("failed to set running status", "error", err)
		return err
	}

	_, uploadErr := c.Upload(ctx, body, &c.Bucket, &filename)
	if uploadErr != nil {
		c.Log.Errorf("error during upload: %v", uploadErr)
		statusError := models.SourceError{Message: uploadErr.Error(), Code: 1} // TODO: determine a better approach to assigning an internal status code
		if err := payload.SetSourceStatus(db, urlparams.ResourceUUID, models.RFailed, &statusError); err != nil {
			c.Log.Errorw("failed to set source status after failed upload", "error", err)
			return uploadErr
		}
		return uploadErr
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
	payload, err := db.Get(uid)
	if err != nil {
		c.Log.Errorf("failed to get payload: %v", err)
		return
	}
	ready, err := payload.GetAllSourcesStatus()
	if err != nil {
		c.Log.Errorf("failed to get all source status: %v", err)
		return
	}
	switch ready {
	case models.StatusComplete, models.StatusPartial:
		if payload.Status == models.Running {
			c.Log.Infow("ready for zipping", "export-uuid", payload.ID)
			go c.compressPayload(db, payload) // start a go-routine to not block
		}
	case models.StatusPending:
		return
	case models.StatusFailed:
		c.Log.Infof("all sources for payload %s reported as failure", payload.ID)
		if err := payload.SetStatusFailed(db); err != nil {
			c.Log.Errorw("failed updating model status after sources failed", "error", err)
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

func (mc *MockStorageHandler) CreateObject(ctx context.Context, db models.DBInterface, body io.Reader, urlparams *models.URLParams, payload *models.ExportPayload) error {
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
