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
	middleware "github.com/go-chi/chi/v5/middleware"
	lc "github.com/redhatinsights/platform-go-middlewares/v2/logging/cloudwatch"
	"github.com/redhatinsights/platform-go-middlewares/v2/request_id"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/redhatinsights/export-service-go/config"
)

// Log is a global variable that carries the Sugared logger
var Log *zap.SugaredLogger

var cfg = config.Get()

func Get() *zap.SugaredLogger {
	if Log == nil {
		tmpLogger := zap.NewExample()
		loggerConfig := zap.NewProductionConfig()
		loggerConfig.EncoderConfig.TimeKey = "@timestamp"
		loggerConfig.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout("2006-01-02T15:04:05.9999Z")

		consoleOutput := zapcore.Lock(os.Stdout)
		consoleEncoder := zapcore.NewJSONEncoder(loggerConfig.EncoderConfig)
		if cfg.Debug {
			// use color and non-JSON logging in DEBUG mode
			loggerConfig.Development = true
			loggerConfig.EncoderConfig.EncodeLevel = zapcore.LowercaseColorLevelEncoder
			consoleEncoder = zapcore.NewConsoleEncoder(loggerConfig.EncoderConfig)
		}

		switch cfg.LogLevel {
		case "DEBUG":
			loggerConfig.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
		case "ERROR":
			loggerConfig.Level = zap.NewAtomicLevelAt(zapcore.ErrorLevel)
		default:
			loggerConfig.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
		}

		fn := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool { return true })
		core := zapcore.NewTee(zapcore.NewCore(consoleEncoder, consoleOutput, fn))

		// configure cloudwatch
		if cfg.Logging != nil && cfg.Logging.Region != "" {
			cred := credentials.NewStaticCredentials(cfg.Logging.AccessKeyID, cfg.Logging.SecretAccessKey, "")
			awsconf := aws.NewConfig().WithRegion(cfg.Logging.Region).WithCredentials(cred)
			batchLogWriter, err := lc.NewBatchWriterWithDuration(cfg.Logging.LogGroup, cfg.Hostname, awsconf, 10*time.Second)
			if err != nil {
				tmpLogger.Info(err.Error())
			}
			hook := zapcore.AddSync(batchLogWriter)
			core = zapcore.NewTee(
				zapcore.NewCore(consoleEncoder, consoleOutput, fn),
				zapcore.NewCore(consoleEncoder, hook, fn),
			)
		}

		logger, err := loggerConfig.Build(zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
		if err != nil {
			tmpLogger.Info(err.Error())
		}

		Log = logger.Sugar()
		Log.Infof("log level set to %s", cfg.LogLevel)
	}
	return Log
}

// ResponseLogger is a middleware that sets the ResponseLogger to the global default.
func ResponseLogger(next http.Handler) http.Handler {
	return SetResponseLogger(Log)(next)
}

// SetResponseLogger is a middleware helper that accepts a configured zap.SugaredLogger
// and logs response information for each API response.
func SetResponseLogger(l *zap.SugaredLogger) func(next http.Handler) http.Handler {
	fn1 := func(next http.Handler) http.Handler {
		fn2 := func(w http.ResponseWriter, r *http.Request) {
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
					"request_id", request_id.GetReqID(r.Context()),
					"user-agent", r.UserAgent(),
				)
			}()
			next.ServeHTTP(ww, r)
		}
		return http.HandlerFunc(fn2)
	}
	return fn1
}

func RequestIDField(requestID string) zap.Field {
	return zap.String("request_id", requestID)
}

func OrgIDField(orgID string) zap.Field {
	return zap.String("org_id", orgID)
}

func ExportIDField(exportID string) zap.Field {
	return zap.String("export_id", exportID)
}
