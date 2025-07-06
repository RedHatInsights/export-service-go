/*
Copyright 2022 Red Hat Inc.
SPDX-License-Identifier: Apache-2.0
*/
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"

	chi "github.com/go-chi/chi/v5"
	middleware "github.com/go-chi/chi/v5/middleware"
	redoc "github.com/go-openapi/runtime/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/redhatinsights/platform-go-middlewares/v2/request_id"
	"go.uber.org/zap"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"

	s3_manager "github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/redhatinsights/export-service-go/config"
	"github.com/redhatinsights/export-service-go/db"
	"github.com/redhatinsights/export-service-go/exports"
	ekafka "github.com/redhatinsights/export-service-go/kafka"
	"github.com/redhatinsights/export-service-go/logger"
	metrics "github.com/redhatinsights/export-service-go/metrics"
	emiddleware "github.com/redhatinsights/export-service-go/middleware"
	"github.com/redhatinsights/export-service-go/models"
	es3 "github.com/redhatinsights/export-service-go/s3"
)

// func serveWeb(cfg *config.ExportConfig, consumers []services.ConsumerService) *http.Server {
func createPublicServer(cfg *config.ExportConfig, external exports.Export) *http.Server {
	// Initialize router
	router := chi.NewRouter()

	// setup middleware
	router.Use(
		request_id.ConfiguredRequestID("x-rh-insights-request-id"),
		emiddleware.JSONContentType, // Set content-Type headers as application/json
		logger.ResponseLogger,
		setupDocsMiddleware,
		metrics.PrometheusMiddleware,
		middleware.Recoverer,
	)

	router.Get("/", statusOK)
	router.Get("/api/export/v1/openapi.json", servePublicOpenAPISpec(cfg)) // OpenAPI Specs
	router.Get("/app/export/v1/openapi.json", servePrivateOpenAPISpec(cfg))

	router.Route("/api/export/v1", func(r chi.Router) {
		// add authentication middleware
		r.Use(
			identity.EnforceIdentity,        // EnforceIdentity extracts the X-Rh-Identity header and places the contents into the request context.
			emiddleware.EnforceUserIdentity, // EnforceUserIdentity extracts account_number, org_id, and username from the X-Rh-Identity context.
		)

		// add external routes
		r.Get("/ping", helloWorld) // Hello World endpoint
		r.Route("/exports", external.ExportRouter)
	})

	server := http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.PublicPort),
		Handler:      router,
		ReadTimeout:  cfg.PublicHttpServerReadTimeout,
		WriteTimeout: cfg.PublicHttpServerWriteTimeout,
	}
	// server.RegisterOnShutdown(func() {
	// 	// initialize Kafka producers/consumers here
	// 	for _, producer := range producers {
	// 		if producer != nil {
	// 			producer.Shutdown()
	// 		}
	// 	}
	// })
	return &server
}

func createPrivateServer(cfg *config.ExportConfig, internal exports.Internal) *http.Server {
	// Initialize router
	router := chi.NewRouter()

	// setup middleware
	router.Use(
		request_id.RequestID,
		emiddleware.JSONContentType, // Set content-Type headers as application/json
		logger.ResponseLogger,
		metrics.PrometheusMiddleware,
		middleware.Recoverer,
	)

	router.Get("/", statusOK)

	router.Route("/app/export/v1", func(r chi.Router) {
		r.Use(emiddleware.EnforcePSK)
		// add internal routes
		r.Get("/ping", helloWorld) // Hello World endpoint
		r.Route("/", internal.InternalRouter)
	})

	return &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.PrivatePort),
		Handler: router,
		// TODO: tune these timeouts. This server is repsonsible for writing to s3.
		// It is possible these values are way too low depending on the dataset received.
		ReadTimeout:  cfg.PrivateHttpServerReadTimeout,
		WriteTimeout: cfg.PrivateHttpServerWriteTimeout,
	}
}

func createMetricsServer(cfg *config.ExportConfig) *http.Server {
	// Router for metrics
	mr := chi.NewRouter()
	mr.Get("/", statusOK)
	mr.Get("/readyz", statusOK)  // for readiness probe
	mr.Get("/healthz", statusOK) // for liveness probe
	mr.Handle("/metrics", promhttp.Handler())

	return &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.MetricsPort),
		Handler: mr,
	}
}

func setupDocsMiddleware(handler http.Handler) http.Handler {
	opt := redoc.RedocOpts{
		SpecURL: "/api/export/v1/openapi.json",
	}
	return redoc.Redoc(opt, handler)
}

// Handler function that responds with Hello World
func helloWorld(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "Hello world")
}

// statusOK returns a simple 200 status code
func statusOK(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
}

// Serve OpenAPI spec json
func servePublicOpenAPISpec(cfg *config.ExportConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, cfg.OpenAPIPublicPath)
	}
}

func servePrivateOpenAPISpec(cfg *config.ExportConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, cfg.OpenAPIPrivatePath)
	}
}

func startApiServer(cfg *config.ExportConfig, log *zap.SugaredLogger) {
	log.Infow("configuration values",
		"hostname", cfg.Hostname,
		"publicport", cfg.PublicPort,
		"public_http_server_read_timeout", cfg.PublicHttpServerReadTimeout,
		"public_http_server_write_timeout", cfg.PublicHttpServerWriteTimeout,
		"private_http_server_read_timeout", cfg.PrivateHttpServerReadTimeout,
		"private_http_server_write_timeout", cfg.PrivateHttpServerWriteTimeout,
		"metricsport", cfg.MetricsPort,
		"loglevel", cfg.LogLevel,
		"debug", cfg.Debug,
		"publicopenapifilepath", cfg.OpenAPIPublicPath,
		"privateopenapifilepath", cfg.OpenAPIPrivatePath,
		"exportableApplications", cfg.ExportableApplications,
		"aws_uploader_buffer_size", cfg.StorageConfig.AwsUploaderBufferSize,
		"aws_downloader_buffer_size", cfg.StorageConfig.AwsDownloaderBufferSize,
	)

	kafkaProducerMessagesChan := make(chan *kafka.Message) // TODO: determine an appropriate buffer (if one is actually necessary)

	producer, err := ekafka.NewProducer()
	if err != nil {
		log.Panic("failed to create kafka producer", "error", err)
	}
	log.Infof("created kafka producer: %s", producer.String())
	go producer.StartProducer(kafkaProducerMessagesChan)

	DB, err := db.OpenDB(*cfg)
	if err != nil {
		log.Panic("failed to open database", "error", err)
	}

	kafkaRequestAppResources := exports.KafkaRequestApplicationResources(kafkaProducerMessagesChan)

	s3Client := es3.NewS3Client(*cfg, log)

	storageHandler := es3.Compressor{
		Bucket: cfg.StorageConfig.Bucket,
		Log:    log,
		Client: *s3Client,
		Cfg:    *cfg,
		Uploader: s3_manager.NewUploader(s3Client, func(u *s3_manager.Uploader) {
			u.PartSize = cfg.StorageConfig.AwsUploaderBufferSize
		}),
		Downloader: s3_manager.NewDownloader(s3Client, func(d *s3_manager.Downloader) {
			d.PartSize = cfg.StorageConfig.AwsDownloaderBufferSize
		}),
	}

	external := exports.Export{
		Bucket:              cfg.StorageConfig.Bucket,
		StorageHandler:      &storageHandler,
		DB:                  &models.ExportDB{DB: DB, Cfg: cfg},
		RequestAppResources: kafkaRequestAppResources,
		Log:                 log,
	}
	wsrv := createPublicServer(cfg, external)

	internal := exports.Internal{
		Cfg:        cfg,
		Compressor: &storageHandler,
		DB:         &models.ExportDB{DB: DB, Cfg: cfg},
		Log:        log,
	}
	psrv := createPrivateServer(cfg, internal)
	msrv := createMetricsServer(cfg)

	idleConnsClosed := make(chan struct{})
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt)
		<-sigint
		if err := wsrv.Shutdown(context.Background()); err != nil {
			log.Errorw("http server shutdown failed", "error", err)
		}
		log.Info("public server shutdown")
		if err := psrv.Shutdown(context.Background()); err != nil {
			log.Errorw("http server shutdown failed", "error", err)
		}
		log.Info("private server shutdown")
		if err := msrv.Shutdown(context.Background()); err != nil {
			log.Errorw("http server shutdown failed", "error", err)
		}
		log.Info("metrics server shutdown")
		close(idleConnsClosed)
	}()

	go func() {
		if err := msrv.ListenAndServe(); err != http.ErrServerClosed {
			log.Errorw("metrics server stopped", "error", err)
		}
	}()
	log.Infof("metrics server started on %s", msrv.Addr)

	go func() {
		if err := wsrv.ListenAndServe(); err != http.ErrServerClosed {
			log.Panicw("public server stopped unexpectedly", "error", err)
		}
	}()
	log.Infof("public server started on %s", wsrv.Addr)

	go func() {
		if err := psrv.ListenAndServe(); err != http.ErrServerClosed {
			log.Panicw("private server stopped unexpectedly", "error", err)
		}
	}()
	log.Infof("private server started on %s", psrv.Addr)

	<-idleConnsClosed

	close(kafkaProducerMessagesChan)

	log.Info("flushing kafka producer")
	producer.Flush(1500) // 1.5 second timeout
	producer.Close()
	log.Info("closed kafka producer")

	log.Info("syncing logger")
	if err := log.Sync(); err != nil {
		log.Errorw("failed to sync logger", "error", err)
	} else {
		log.Info("synced logger")
	}

	log.Info("everything has shut down, goodbye")
}
