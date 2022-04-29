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

	"github.com/redhatinsights/export-service-go/config"
	"github.com/redhatinsights/export-service-go/db"
	"github.com/redhatinsights/export-service-go/models"
)

var (
	// cfg    = config.ExportCfg.StorageConfig
	s3Chan = config.ExportCfg.Channels.ToS3Chan
)

type Compressor struct{}

func (c *Compressor) Start() {
	log.Infof("started s3 compressor")
	for m := range s3Chan {
		go func(m *models.ExportPayload) {
			log.Infof("starting payload compression for %s", m.ID)
			bucket := cfg.StorageConfig.Bucket
			prefix := fmt.Sprintf("%s/%s/", m.OrganizationID, m.ID)

			paginator := s3.NewListObjectsV2Paginator(Client, &s3.ListObjectsV2Input{
				Bucket: &bucket,
				Prefix: &prefix,
			})

			downloader := manager.NewDownloader(Client)

			var buf bytes.Buffer
			gzipWriter := gzip.NewWriter(&buf)
			tarWriter := tar.NewWriter(gzipWriter)

			for paginator.HasMorePages() {
				page, err := paginator.NextPage(context.TODO())
				if err != nil {
					log.Errorw("failed to get next page of data", "error", err)
					return
				}
				for _, obj := range page.Contents {
					log.Infof("downloading s3://%s/%s...", bucket, *obj.Key)
					basename := filepath.Base(*obj.Key)
					// filename := strings.TrimSuffix(basename, filepath.Ext(basename))
					f, err := os.CreateTemp("", basename)
					if err != nil {
						log.Errorw("failed to create temp file", "error", err)
						return
					}
					if _, err := downloader.Download(context.TODO(), f, &s3.GetObjectInput{Bucket: &bucket, Key: obj.Key}); err != nil {
						log.Errorw("failed to download to file", "error", err)
						return
					}
					fi, err := f.Stat()
					if err != nil {
						log.Errorw("failed to get file info", "error", err)
						return
					}
					header, err := tar.FileInfoHeader(fi, basename)
					if err != nil {
						log.Errorw("failed to create file header", "error", err)
						return
					}
					header.Name = basename

					if err = tarWriter.WriteHeader(header); err != nil {
						log.Errorw("failed to write header", "error", err)
						return
					}
					if _, err := io.Copy(tarWriter, f); err != nil {
						log.Errorw("failed to copy data into tar file", "error", err)
						return
					}
					log.Infof("added file %s to payload", basename)
				}
			}

			// produce tar
			if err := tarWriter.Close(); err != nil {
				log.Errorw("failed to close tar writer", "error", err)
				return
			}
			// produce gzip
			if err := gzipWriter.Close(); err != nil {
				log.Errorw("failed to close gzip writer", "error", err)
				return
			}

			t := time.Now()
			filename := fmt.Sprintf("%s-%s.tar.gz", m.ID.String(), t.Format(time.RFC3339))
			// target := filepath.Join("./tmp", filename)
			f, err := os.CreateTemp("", filename)
			if err != nil {
				log.Errorw("failed to create temp file", "error", err)
				return
			}

			if _, err := io.Copy(f, &buf); err != nil {
				panic(err)
			}

			log.Infof("saving temp file %s", filename)
			log.Infof("shipping %s to s3", filename)

			// seek to the beginning of the file so that we can reuse the file handler for upload
			if _, err := f.Seek(0, 0); err != nil {
				log.Errorf("failed to seek to beginning of file", "error", err)
				return
			}

			// upload zip to s3
			s3Filename := fmt.Sprintf("%s/%s", m.OrganizationID, filename)

			if _, err := Upload(context.TODO(), f, &cfg.StorageConfig.Bucket, &s3Filename); err != nil {
				log.Errorw("failed to upload tarfile to s3", "error", err, "filename", filename)
				return
			}

			log.Infof("done uploading %s", filename)

			m.Status = models.Complete
			m.CompletedAt = &t            // match the filename time to the db entry
			m.S3DownloadLink = s3Filename // TODO: maybe we should rename this field to S3Key or something
			if err := db.DB.Save(m).Error; err != nil {
				log.Errorw("failed updating model status after upload", "error", err)
				return
			}

		}(m)
	}
}

func Upload(c context.Context, body io.Reader, bucket, key *string) (*manager.UploadOutput, error) {
	uploader := manager.NewUploader(Client, func(u *manager.Uploader) {
		u.PartSize = 100 * 1024 * 1024 // 100 MiB
	})

	input := &s3.PutObjectInput{
		Bucket: bucket,
		Key:    key,
		Body:   body,
	}
	return uploader.Upload(c, input)
}
