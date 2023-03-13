package s3

import (
	"github.com/prometheus/client_golang/prometheus"
)

var failUploads = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "failed_s3_uploads",
	Help: "The total number of failed S3 uploads.",
})

func init() {
	prometheus.MustRegister(failUploads)
}
