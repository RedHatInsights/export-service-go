package s3

import (
	"github.com/prometheus/client_golang/prometheus"
)

var totalUploads = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "total_s3_uploads",
	Help: "The total number of S3 uploads, including failed uploads.",
})

var failUploads = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "failed_s3_uploads",
	Help: "The total number of failed S3 uploads.",
})

func init() {
	prometheus.MustRegister(totalUploads)
	prometheus.MustRegister(failUploads)
}
