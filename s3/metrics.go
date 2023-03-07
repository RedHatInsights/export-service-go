package s3

import (
	"github.com/prometheus/client_golang/prometheus"
)

var failUploads = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "fail_uploads_total",
	Help: "The total number of failed S3 uploads.",
})

func init() {
	prometheus.MustRegister(failUploads)
}
