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
	"time"

	chi "github.com/go-chi/chi/v5"
	middleware "github.com/go-chi/chi/v5/middleware"
	redoc "github.com/go-openapi/runtime/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	metrics "github.com/redhatinsights/export-service-go/metrics"
	"github.com/redhatinsights/platform-go-middlewares/identity"
	"github.com/redhatinsights/platform-go-middlewares/request_id"
	"go.uber.org/zap"

	"github.com/redhatinsights/export-service-go/config"
	"github.com/redhatinsights/export-service-go/exports"
	"github.com/redhatinsights/export-service-go/logger"
	emiddleware "github.com/redhatinsights/export-service-go/middleware"
)

var cfg *config.ExportConfig
var log *zap.SugaredLogger

func init() {
	cfg = config.ExportCfg
	log = logger.Log
}

// func serveWeb(cfg *config.ExportConfig, consumers []services.ConsumerService) *http.Server {
func createWebServer(cfg *config.ExportConfig) *http.Server {
	// Initialize router
	router := chi.NewRouter()

	// setup middleware
	router.Use(
		request_id.RequestID,
		emiddleware.JSONContentType, // Set content-Type headers as application/json
		logger.ResponseLogger,
		setupDocsMiddleware,
		metrics.PrometheusMiddleware,
		middleware.Recoverer,
	)

	router.Get("/", statusOK)

	router.Route("/api/export/v1", func(r chi.Router) {
		// add authentication middleware
		r.Use(
			emiddleware.InjectDebugUserIdentity, // InjectDebugUserIdentity injects a valid X-Rh-Identity header when the config.Debug is true.
			identity.EnforceIdentity,            // EnforceIdentity extracts the X-Rh-Identity header and places the contents into the request context.
			emiddleware.EnforceUserIdentity,     // EnforceUserIdentity extracts account_number, org_id, and username from the X-Rh-Identity context.
		)

		// add external routes
		r.Get("/openapi.json", serveOpenAPISpec) // OpenAPI Spec
		r.Get("/ping", helloWorld)               // Hello World endpoint
		r.Route("/exports", exports.ExportRouter)
	})

	server := http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.WebPort),
		Handler:      router,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	server.RegisterOnShutdown(func() {
		// initialize Kafka producers/consumers here
		// for _, consumer := range consumers {
		// 	if consumer != nil {
		// 		consumer.Close()
		// 	}
		// }
	})
	return &server
}

func createPrivateServer(cfg *config.ExportConfig) *http.Server {
	// Initialize router
	router := chi.NewRouter()

	// setup middleware
	router.Use(
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
		r.Route("/", exports.InternalRouter)
	})

	return &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.PrivatePort),
		Handler: router,
		// TODO: tune these timeouts. This server is repsonsible for writing to s3.
		// It is possible these values are way too low depending on the dataset received.
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
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
func serveOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, cfg.OpenAPIFilePath)
}

func main() {
	log.Infow("configuration values",
		"hostname", cfg.Hostname,
		"webport", cfg.WebPort,
		"metricsport", cfg.MetricsPort,
		"loglevel", cfg.LogLevel,
		"debug", cfg.Debug,
		"openapifilepath", cfg.OpenAPIFilePath,
		"psks", cfg.Psks, // TODO: remove this
	)

	wsrv := createWebServer(cfg)

	psrv := createPrivateServer(cfg)

	msrv := createMetricsServer(cfg)

	idleConnsClosed := make(chan struct{})
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt)
		<-sigint
		if err := wsrv.Shutdown(context.Background()); err != nil {
			log.Errorw("http server shutdown failed", "error", err)
		}
		log.Info("web server shutdown")
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
			log.Panicw("web server stopped unexpectedly", "error", err)
		}
	}()
	log.Infof("web server started on %s", wsrv.Addr)

	go func() {
		if err := psrv.ListenAndServe(); err != http.ErrServerClosed {
			log.Panicw("private server stopped unexpectedly", "error", err)
		}
	}()
	log.Infof("private server started on %s", psrv.Addr)

	<-idleConnsClosed
	log.Info("everything has shut down, goodbye")
}
