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
	"github.com/go-chi/render"
	redoc "github.com/go-openapi/runtime/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	metrics "github.com/redhatinsights/export-service-go/metrics"
	"github.com/redhatinsights/platform-go-middlewares/identity"
	"github.com/redhatinsights/platform-go-middlewares/request_id"
	"go.uber.org/zap"

	"github.com/redhatinsights/export-service-go/config"
	"github.com/redhatinsights/export-service-go/exports"
	"github.com/redhatinsights/export-service-go/logging"
)

var log *zap.SugaredLogger
var cfg *config.ExportConfig

func init() {
	cfg = config.ExportCfg
	log = logging.Log
}

// func serveWeb(cfg *config.ExportConfig, consumers []services.ConsumerService) *http.Server {
func serveWeb(cfg *config.ExportConfig) *http.Server {
	server := http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.WebPort),
		Handler:      webRoutes(),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	server.RegisterOnShutdown(func() {
		// for _, consumer := range consumers {
		// 	if consumer != nil {
		// 		consumer.Close()
		// 	}
		// }
	})
	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Panicf("web service stopped unexpectedly: %v", err)
		}
	}()
	log.Info("web service started")
	return &server
}

func webRoutes() *chi.Mux {
	router := chi.NewRouter()
	router.Use(
		request_id.ConfiguredRequestID("x-rh-insights-request-id"),
		render.SetContentType(render.ContentTypeJSON), // Set content-Type headers as application/json
		logging.Logger(log),
		middleware.Recoverer,
	)

	router.Route("/api/export/v1", func(r chi.Router) {
		r.Route("/exports", exports.ExportRouter)
	})

	router.HandleFunc("/hello/{name}", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "hello, %s!\n", chi.URLParam(r, "name"))
	})

	return router
}

// func main() {
// 	interruptSignal := make(chan os.Signal, 1)
// 	signal.Notify(interruptSignal, os.Interrupt, syscall.SIGTERM)

// 	serveWeb(cfg)

// 	// block here and shut things down on interrupt
// 	<-interruptSignal
// 	log.Info("Shutting down gracefully...")
// 	// temporarily adding a sleep to help troubleshoot interrupts

// }

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
	// log.WithFields(log.Fields{
	// 	"Hostname":         cfg.Hostname,
	// 	"Auth":             cfg.Auth,
	// 	"WebPort":          cfg.WebPort,
	// 	"MetricsPort":      cfg.MetricsPort,
	// 	"LogLevel":         cfg.LogLevel,
	// 	"Debug":            cfg.Debug,
	// 	"OpenAPIFilePath ": cfg.OpenAPIFilePath,
	// }).Info("Configuration Values:")

	// Initialize router
	r := chi.NewRouter()

	r.Use(
		middleware.Logger,
		setupDocsMiddleware,
		metrics.PrometheusMiddleware,
	)

	// Register handler functions on server routes

	// Unauthenticated routes
	r.Get("/", statusOK)                                   // Health check
	r.Get("/api/export/v1/openapi.json", serveOpenAPISpec) // OpenAPI Spec

	// Authenticated routes
	ar := r.Group(nil)
	if cfg.Auth {
		ar.Use(identity.EnforceIdentity) // EnforceIdentity extracts the X-Rh-Identity header and places the contents into the request context.
	}

	ar.Get("/api/export/v1/ping", helloWorld) // Hello World endpoint

	// Router for metrics
	mr := chi.NewRouter()
	mr.Get("/", statusOK)
	mr.Handle("/metrics", promhttp.Handler())

	srv := http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.WebPort),
		Handler: r,
	}

	msrv := http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.MetricsPort),
		Handler: mr,
	}

	idleConnsClosed := make(chan struct{})
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt)
		<-sigint
		if err := srv.Shutdown(context.Background()); err != nil {
			log.Errorw("http server shutdown failed", "error", err)
		}
		if err := msrv.Shutdown(context.Background()); err != nil {
			log.Errorw("http server shutdown failed", "error", err)
		}
		close(idleConnsClosed)
	}()

	go func() {
		if err := msrv.ListenAndServe(); err != http.ErrServerClosed {
			log.Errorw("metrics service stopped", "error", err)
		}
	}()

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Errorw("service stopped", "error", err)
	}

	<-idleConnsClosed
	log.Info("Everything has shut down, goodbye")
}
