package s3

import "github.com/redhatinsights/export-service-go/config"

var s3Chan = config.ExportCfg.Channels.ToS3Chan
