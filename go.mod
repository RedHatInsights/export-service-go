module github.com/redhatinsights/export-service-go

go 1.16

require (
	github.com/aws/aws-sdk-go v1.38.51
	github.com/aws/aws-sdk-go-v2 v1.16.2
	github.com/aws/aws-sdk-go-v2/feature/s3/manager v1.11.5
	github.com/aws/aws-sdk-go-v2/service/s3 v1.26.5
	github.com/confluentinc/confluent-kafka-go v1.8.2
	github.com/go-chi/chi/v5 v5.0.7
	github.com/go-openapi/runtime v0.23.3
	github.com/google/uuid v1.3.0
	github.com/onsi/ginkgo/v2 v2.3.1
	github.com/onsi/gomega v1.22.1
	github.com/prometheus/client_golang v1.12.1
	github.com/redhatinsights/app-common-go v1.6.0
	github.com/redhatinsights/platform-go-middlewares v0.12.0
	github.com/spf13/viper v1.11.0
	go.uber.org/zap v1.21.0
	gorm.io/datatypes v1.0.6
	gorm.io/driver/postgres v1.3.4
	gorm.io/gorm v1.23.4
)
