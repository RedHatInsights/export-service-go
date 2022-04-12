/*

Copyright 2022 Red Hat Inc.
SPDX-License-Identifier: Apache-2.0

*/
package main

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/render"
	"github.com/redhatinsights/platform-go-middlewares/request_id"
	"go.uber.org/zap"

	"github.com/maskarb/export-service-go/config"
	"github.com/maskarb/export-service-go/exports"
	"github.com/maskarb/export-service-go/logging"
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

func main() {
	interruptSignal := make(chan os.Signal, 1)
	signal.Notify(interruptSignal, os.Interrupt, syscall.SIGTERM)

	serveWeb(cfg)

	// block here and shut things down on interrupt
	<-interruptSignal
	log.Info("Shutting down gracefully...")
	// temporarily adding a sleep to help troubleshoot interrupts
}
