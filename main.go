// Copyright Red Hat
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
	"github.com/redhatinsights/export-service-go/config"
	l "github.com/redhatinsights/export-service-go/logger"
	metrics "github.com/redhatinsights/export-service-go/metrics"
	"github.com/redhatinsights/platform-go-middlewares/identity"
	log "github.com/sirupsen/logrus"
)

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
	cfg := config.Get()
	http.ServeFile(w, r, cfg.OpenAPIFilePath)
}

func initDependencies() {
	config.Init()
	l.InitLogger()
}

func main() {
	initDependencies()
	cfg := config.Get()
	log.WithFields(log.Fields{
		"Hostname":         cfg.Hostname,
		"Auth":             cfg.Auth,
		"WebPort":          cfg.WebPort,
		"MetricsPort":      cfg.MetricsPort,
		"LogLevel":         cfg.LogLevel,
		"Debug":            cfg.Debug,
		"OpenAPIFilePath ": cfg.OpenAPIFilePath,
	}).Info("Configuration Values:")

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
			log.WithFields(log.Fields{"error": err}).Fatal("HTTP Server Shutdown failed")
		}
		if err := msrv.Shutdown(context.Background()); err != nil {
			log.WithFields(log.Fields{"error": err}).Fatal("HTTP Server Shutdown failed")
		}
		close(idleConnsClosed)
	}()

	go func() {
		if err := msrv.ListenAndServe(); err != http.ErrServerClosed {
			log.WithFields(log.Fields{"error": err}).Fatal("Metrics Service Stopped")
		}
	}()

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.WithFields(log.Fields{"error": err}).Fatal("Service Stopped")
	}

	<-idleConnsClosed
	log.Info("Everything has shut down, goodbye")
}
