package s3

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"go.uber.org/zap"

	"github.com/redhatinsights/export-service-go/models"
)

type Compressor struct {
	Bucket string
	Log    *zap.SugaredLogger
}

// S3ListObjectsAPI defines the interface for the ListObjectsV2 function.
// We use this interface to test the function using a mocked service.
// https://aws.github.io/aws-sdk-go-v2/docs/code-examples/s3/listobjects/
type S3ListObjectsAPI interface {
	ListObjectsV2(ctx context.Context,
		params *s3.ListObjectsV2Input,
		optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
}

func GetObjects(c context.Context, api S3ListObjectsAPI, input *s3.ListObjectsV2Input) (*s3.ListObjectsV2Output, error) {
	return api.ListObjectsV2(c, input)
}

func (c *Compressor) zipExport(ctx context.Context, t time.Time, prefix, filename, s3key string) error {
	input := &s3.ListObjectsV2Input{
		Bucket: &c.Bucket,
		Prefix: &prefix,
	}

	var buf bytes.Buffer
	gzipWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzipWriter)

	// resp, err := Client.ListObjectsV2(ctx, input)
	resp, err := GetObjects(ctx, Client, input)
	if err != nil {
		return fmt.Errorf("failed to list bucket objects: %v", err)
	}

	for _, obj := range resp.Contents {

		c.Log.Infof("downloading s3://%s/%s...", c.Bucket, *obj.Key)
		basename := filepath.Base(*obj.Key)
		f, err := os.CreateTemp("", basename)
		if err != nil {
			return fmt.Errorf("failed to create temp file: %v", err)
		}
		if _, err := c.Download(ctx, f, &c.Bucket, obj.Key); err != nil {
			return fmt.Errorf("failed to download to file: %v", err)
		}
		fi, err := f.Stat()
		if err != nil {
			return fmt.Errorf("failed to get file info: %v", err)
		}
		header, err := tar.FileInfoHeader(fi, basename)
		if err != nil {
			return fmt.Errorf("failed to create file header: %v", err)
		}
		header.Name = basename

		if err = tarWriter.WriteHeader(header); err != nil {
			return fmt.Errorf("failed to write header: %v", err)
		}
		if _, err := io.Copy(tarWriter, f); err != nil {
			return fmt.Errorf("failed to copy data into tar file: %v", err)
		}
		c.Log.Infof("added file %s to payload", basename)
	}

	// produce tar
	if err := tarWriter.Close(); err != nil {
		return fmt.Errorf("failed to close tar writer: %v", err)
	}
	// produce gzip
	if err := gzipWriter.Close(); err != nil {
		return fmt.Errorf("failed to close gzip writer: %v", err)
	}

	f, err := os.CreateTemp("", filename)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %v", err)
	}

	if _, err := io.Copy(f, &buf); err != nil {
		return fmt.Errorf("failed to copy buffer into file: %v", err)
	}

	c.Log.Infof("saving temp file %s", filename)
	c.Log.Infof("shipping %s to s3", filename)

	// seek to the beginning of the file so that we can reuse the file handler for upload
	if _, err := f.Seek(0, 0); err != nil {
		return fmt.Errorf("failed to seek to beginning of file: %v", err)
	}

	if _, err := c.Upload(ctx, f, &cfg.StorageConfig.Bucket, &s3key); err != nil {
		return fmt.Errorf("failed to upload tarfile `%s` to s3: %v", s3key, err)
	}

	return nil
}

func (c *Compressor) Compress(ctx context.Context, m *models.ExportPayload) (time.Time, string, string, error) {
	t := time.Now()

	c.Log.Infof("starting payload compression for %s", m.ID)
	prefix := fmt.Sprintf("%s/%s/", m.OrganizationID, m.ID)
	filename := fmt.Sprintf("%s-%s.tar.gz", m.ID.String(), t.Format(time.RFC3339))
	s3key := fmt.Sprintf("%s/%s", m.OrganizationID, filename)

	err := c.zipExport(ctx, t, prefix, filename, s3key)
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
