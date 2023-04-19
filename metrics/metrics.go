/*
Copyright 2022 Red Hat Inc.
SPDX-License-Identifier: Apache-2.0
*/
package metrics

import (
	"net/http"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

var httpReqs = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "export_service_http_requests_total",
		Help: "How many HTTP requests processed, partitioned by status code, http method and path.",
	},
	[]string{"code", "method", "path"},
)

var httpDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name: "export_service_http_response_time_seconds",
	Help: "Duration of HTTP requests, partitioned by path.",
}, []string{"path"})

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func NewResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{w, http.StatusOK}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func PrometheusMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		timer := prometheus.NewTimer(httpDuration.WithLabelValues(r.URL.Path))
		rw := NewResponseWriter(w)
		next.ServeHTTP(rw, r)

		statusCode := rw.statusCode

		httpReqs.WithLabelValues(strconv.Itoa(statusCode), r.Method, r.URL.Path).Inc()

		timer.ObserveDuration()
	})
}

func init() {
	prometheus.MustRegister(httpReqs)
	prometheus.MustRegister(httpDuration)
}
