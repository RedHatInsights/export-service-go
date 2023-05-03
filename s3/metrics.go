package s3

import (
	"github.com/prometheus/client_golang/prometheus"
)

var totalUploads = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "export_service_total_s3_uploads",
	Help: "The total number of S3 uploads, including failed uploads.",
})

var failUploads = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "export_service_failed_s3_uploads",
	Help: "The total number of failed S3 uploads.",
})

var uploadSizes = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name: "export_service_upload_sizes",
	Help: "Size of payloads posted",
}, []string{"account", "org_id", "app"})

func init() {
	prometheus.MustRegister(totalUploads)
	prometheus.MustRegister(failUploads)
	prometheus.MustRegister(uploadSizes)
	// Set an initial value of 0 for the histogram so that it shows up in the metrics
	uploadSizes.With(prometheus.Labels{"account": "testAccount", "org_id": "testOrg", "app": "testApp"})
}
