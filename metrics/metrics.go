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
		Help: "How many HTTP requests processed, partitioned by status code and http method.",
	},
	[]string{"code", "method"},
)

var httpDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name: "export_service_http_response_time_seconds",
	Help: "Duration of HTTP requests.",
}, []string{"method"})

var oidcProviderInitDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
	Name: "export_service_oidc_provider_init_duration_seconds",
	Help: "Duration of OIDC provider initialization.",
})

var oidcProviderInitFailures = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "export_service_oidc_provider_init_failures_total",
	Help: "Total number of OIDC provider initialization failures.",
})

var oidcVerificationDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
	Name: "export_service_oidc_verification_duration_seconds",
	Help: "Duration of OIDC token verification.",
})

var oidcVerificationFailures = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "export_service_oidc_verification_failures_total",
		Help: "Total number of OIDC token verification failures.",
	},
	[]string{"reason"},
)

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
		timer := prometheus.NewTimer(httpDuration.WithLabelValues(r.Method))
		rw := NewResponseWriter(w)
		next.ServeHTTP(rw, r)

		statusCode := rw.statusCode

		httpReqs.WithLabelValues(strconv.Itoa(statusCode), r.Method).Inc()

		timer.ObserveDuration()
	})
}

func init() {
	prometheus.MustRegister(httpReqs)
	prometheus.MustRegister(httpDuration)
	prometheus.MustRegister(oidcProviderInitDuration)
	prometheus.MustRegister(oidcProviderInitFailures)
	prometheus.MustRegister(oidcVerificationDuration)
	prometheus.MustRegister(oidcVerificationFailures)
}

// ObserveOIDCProviderInit records the duration of OIDC provider initialization
func ObserveOIDCProviderInit(duration float64) {
	oidcProviderInitDuration.Observe(duration)
}

// IncrementOIDCProviderInitFailures increments the OIDC provider initialization failure counter
func IncrementOIDCProviderInitFailures() {
	oidcProviderInitFailures.Inc()
}

// ObserveOIDCVerification records the duration of OIDC token verification
func ObserveOIDCVerification(duration float64) {
	oidcVerificationDuration.Observe(duration)
}

// IncrementOIDCVerificationFailures increments the OIDC verification failure counter
func IncrementOIDCVerificationFailures(reason string) {
	oidcVerificationFailures.WithLabelValues(reason).Inc()
}
