/*

Copyright 2022 Red Hat Inc.
SPDX-License-Identifier: Apache-2.0

*/
package logger

import (
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/go-chi/chi/middleware"
	lc "github.com/redhatinsights/platform-go-middlewares/logging/cloudwatch"
	"github.com/redhatinsights/platform-go-middlewares/request_id"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/redhatinsights/export-service-go/config"
)

var Log *zap.SugaredLogger
var cfg = config.ExportCfg

func init() {
	loggerConfig := zap.NewProductionConfig()
	fn := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return true
	})
	loggerConfig.EncoderConfig.TimeKey = "@timestamp"
	loggerConfig.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout("2006-01-02T15:04:05.999Z")

	consoleOutput := zapcore.Lock(os.Stdout)
	consoleEncoder := zapcore.NewJSONEncoder(loggerConfig.EncoderConfig)

	switch cfg.LogLevel {
	case "DEBUG":
		loggerConfig.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	case "ERROR":
		loggerConfig.Level = zap.NewAtomicLevelAt(zapcore.ErrorLevel)
	default:
		loggerConfig.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	}

	core := zapcore.NewTee(zapcore.NewCore(consoleEncoder, consoleOutput, fn))

	// configure cloudwatch
	if cfg.Logging != nil && cfg.Logging.Region != "" {
		cred := credentials.NewStaticCredentials(cfg.Logging.AccessKeyID, cfg.Logging.SecretAccessKey, "")
		awsconf := aws.NewConfig().WithRegion(cfg.Logging.Region).WithCredentials(cred)
		hook, err := lc.NewBatchingHook(cfg.Logging.LogGroup, cfg.Hostname, awsconf, 10*time.Second)
		if err != nil {
			Log.Info(err)
		}
		core = zapcore.NewTee(
			zapcore.NewCore(consoleEncoder, consoleOutput, fn),
			zapcore.NewCore(consoleEncoder, hook, fn),
		)
	}

	logger, err := loggerConfig.Build(zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
	if err != nil {
		Log.Info(err)
	}

	Log = logger.Sugar()

}

func Logger(l *zap.SugaredLogger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			t1 := time.Now()
			defer func() {
				l.Infow("",
					"protocol", r.Proto,
					"request", r.Method,
					"path", r.URL.Path,
					"latency", time.Since(t1),
					"status", ww.Status(),
					"size", ww.BytesWritten(),
					"reqId", request_id.GetReqID(r.Context()))
			}()

			next.ServeHTTP(ww, r)
		}
		return http.HandlerFunc(fn)
	}
}
