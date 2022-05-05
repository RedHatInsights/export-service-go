module github.com/redhatinsights/export-service-go

go 1.16

require (
	github.com/aws/aws-sdk-go v1.43.38
	github.com/confluentinc/confluent-kafka-go v1.8.2
	github.com/go-chi/chi/v5 v5.0.7
	github.com/go-openapi/runtime v0.23.3
	github.com/google/uuid v1.3.0
	github.com/prometheus/client_golang v1.12.1
	github.com/redhatinsights/app-common-go v1.6.0
	github.com/redhatinsights/platform-go-middlewares v0.12.0
	github.com/spf13/viper v1.11.0
	go.uber.org/zap v1.21.0
	gorm.io/datatypes v1.0.6
	gorm.io/driver/postgres v1.3.4
	gorm.io/gorm v1.23.4
)

require go.uber.org/multierr v1.7.0 // indirect
