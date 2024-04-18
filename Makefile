
OS := $(shell uname)
UNAME_S := $(shell uname -s)
OS_SED :=
ifeq ($(UNAME_S),Darwin)
	OS_SED += ""
endif

OCI_TOOL=$(shell command -v podman || command -v docker)
DOCKER_COMPOSE = $(OCI_TOOL)-compose

CONTAINER_TAG="quay.io/cloudservices/export-service-go"

IDENTITY_HEADER="eyJpZGVudGl0eSI6IHsiYWNjb3VudF9udW1iZXIiOiAiYWNjb3VudDEyMyIsICJvcmdfaWQiOiAib3JnMTIzIiwgInR5cGUiOiAiVXNlciIsICJ1c2VyIjogeyJpc19vcmdfYWRtaW4iOiB0cnVlLCAidXNlcm5hbWUiOiAiZnJlZCJ9LCAiaW50ZXJuYWwiOiB7Im9yZ19pZCI6ICJvcmcxMjMifX19"


help:
	@echo "Please use \`make <target>' where <target> is one of:"
	@echo ""
	@echo "--- General Commands ---"
	@echo "help                     show this message"
	@echo "lint                     runs go lint on the project"
	@echo "vet                      runs go vet on the project"
	@echo "build                    builds the container image"
	@echo "spec                     convert the openapi spec yaml to json"
	@echo "docker-up-db             start the export-service postgres db"
	@echo ""


vet:
	go vet

lint:
	golint

build:
	$(OCI_TOOL) build . -t $(CONTAINER_TAG)

build-local:
	go build -o export-service cmd/export-service/*.go

spec:
ifeq (, $(shell which yq))
	echo "yq is not installed"
else
	@yq -o=json eval static/spec/openapi.yaml > static/spec/openapi.json
	@yq -o=json eval static/spec/private.yaml > static/spec/private.json
endif

docker-down:
	$(DOCKER_COMPOSE) down --remove-orphans

docker-up-db:
	$(DOCKER_COMPOSE) up -d db
	@until pg_isready -h $${POSTGRES_SQL_SERVICE_HOST:-localhost} -p $${POSTGRES_SQL_SERVICE_PORT:-5432} >/dev/null ; do \
		printf '.'; \
		sleep 0.5 ; \
	done

docker-up-no-server: docker-up-db
	$(DOCKER_COMPOSE) up -d kafka s3 s3-createbucket

monitor-topic:
	$(OCI_TOOL) exec -ti kafka /usr/bin/kafka-console-consumer --bootstrap-server localhost:9092 --topic platform.export.requests

run-api: build-local migrate_db
	KAFKA_BROKERS=localhost:9092 DEBUG=true MINIO_PORT=9099 AWS_ACCESS_KEY=minio AWS_SECRET_ACCESS_KEY=minioadmin PSKS=testing-a-psk PUBLIC_PORT=8000 METRICS_PORT=9090 PRIVATE_PORT=10010 PGSQL_PORT=5432 ./export-service api_server

migrate_db: build-local
	PGSQL_PORT=5432 ./export-service migrate_db upgrade

run: docker-up-no-server run-api

sample-request-create-export:
	@curl -sS -X POST http://localhost:8000/api/export/v1/exports -H "x-rh-identity: ${IDENTITY_HEADER}" -H "Content-Type: application/json" -d @example_export_request.json > response.json
	@cat response.json | jq
	@cat response.json | jq -r '.id' | xargs -I {} echo "EXPORT_ID: {}"
	@cat response.json | jq -r '.sources[] | "EXPORT_APPLICATION: \(.application)\nEXPORT_RESOURCE: \(.id)\n---"'
	@rm response.json

sample-request-get-exports:
	curl -X GET http://localhost:8000/api/export/v1/exports -H "x-rh-identity: ${IDENTITY_HEADER}" | jq

# set the variables below based your info from the request above
EXPORT_ID=5a252691-b241-4ef5-a0c4-4b64b96faf61
EXPORT_APPLICATION=exampleApplication
EXPORT_RESOURCE=ee3453cb-eb84-4258-b5f4-228c0fc73719
sample-request-export-status:
	curl -X GET http://localhost:8000/api/export/v1/exports/$(EXPORT_ID)/status -H "x-rh-identity: ${IDENTITY_HEADER}" | jq

sample-request-export-download:
	curl -X GET http://localhost:8000/api/export/v1/exports/$(EXPORT_ID) -H "x-rh-identity: ${IDENTITY_HEADER}" -f --output ./export_download.zip

sample-request-internal-upload:
	curl -X POST http://localhost:10010/app/export/v1/${EXPORT_ID}/${EXPORT_APPLICATION}/${EXPORT_RESOURCE}/upload -H "x-rh-exports-psk: testing-a-psk" -H "Content-Type: application/json" -d @example_export_upload.json

sample-request-internal-error:
	curl -X POST http://localhost:10010/app/export/v1/${EXPORT_ID}/${EXPORT_APPLICATION}/${EXPORT_RESOURCE}/error -H "x-rh-exports-psk: testing-a-psk" -H "Content-Type: application/json" -d @example_export_error.json

make test:
	ginkgo -r --race --randomize-all --randomize-suites

test-sql:
	go test ./... -tags=sql -count=1
