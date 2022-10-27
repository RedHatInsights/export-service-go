
OS := $(shell uname)
UNAME_S := $(shell uname -s)
OS_SED :=
ifeq ($(UNAME_S),Darwin)
	OS_SED += ""
endif

OCI_TOOL=$(shell command -v podman || command -v docker)
DOCKER_COMPOSE = $(OCI_TOOL)-compose

CONTAINER_TAG="quay.io/cloudservices/export-service-go"

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

spec:
ifeq (, $(shell which yq))
	echo "yq is not installed"
else
	yq -o=json eval static/spec/openapi.yaml > static/spec/openapi.json
endif

docker-down:
	$(DOCKER_COMPOSE) down --remove-orphans

docker-up-db:
	$(DOCKER_COMPOSE) up -d db
	@until pg_isready -h $${POSTGRES_SQL_SERVICE_HOST:-localhost} -p $${POSTGRES_SQL_SERVICE_PORT:-15433} >/dev/null ; do \
		printf '.'; \
		sleep 0.5 ; \
	done

docker-up-no-server: docker-up-db
	$(DOCKER_COMPOSE) up -d kafka s3

monitor-topic:
	$(OCI_TOOL) exec -ti kafka /usr/bin/kafka-console-consumer --bootstrap-server localhost:9092 --topic platform.export.requests

run:
	DEBUG=true MINIO_PORT=9099 AWS_ACCESS_KEY=minio AWS_SECRET_ACCESS_KEY=minioadmin PSKS=testing-a-psk PUBLICPORT=8000 METRICSPORT=9090 PRIVATEPORT=10010 PGSQL_PORT=5432 go run main.go

sample-request-create-export:
	curl -X POST http://localhost:8000/api/export/v1/exports -H "x-rh-identity: eyJpZGVudGl0eSI6IHsiYWNjb3VudF9udW1iZXIiOiJhY2NvdW50MTIzIiwib3JnX2lkIjoib3JnMTIzIiwidHlwZSI6IlVzZXIiLCJ1c2VyIjp7ImlzX29yZ19hZG1pbiI6dHJ1ZX0sImludGVybmFsIjp7Im9yZ19pZCI6Im9yZzEyMyJ9fX0K" -H "Content-Type: application/json" -d @example_export_request.json

sample-request-get-exports:
	curl -X GET http://localhost:8000/api/export/v1/exports -H "x-rh-identity: eyJpZGVudGl0eSI6IHsiYWNjb3VudF9udW1iZXIiOiJhY2NvdW50MTIzIiwib3JnX2lkIjoib3JnMTIzIiwidHlwZSI6IlVzZXIiLCJ1c2VyIjp7ImlzX29yZ19hZG1pbiI6dHJ1ZX0sImludGVybmFsIjp7Im9yZ19pZCI6Im9yZzEyMyJ9fX0K"

# set the variables below based your info from the request above
EXPORT_ID=5a252691-b241-4ef5-a0c4-4b64b96faf61
EXPORT_APPLICATION=exampleApplication
EXPORT_RESOURCE=ee3453cb-eb84-4258-b5f4-228c0fc73719
sample-request-export-status:
	curl -X GET http://localhost:8000/api/export/v1/exports/$(EXPORT_ID)/status -H "x-rh-identity: eyJpZGVudGl0eSI6IHsiYWNjb3VudF9udW1iZXIiOiJhY2NvdW50MTIzIiwib3JnX2lkIjoib3JnMTIzIiwidHlwZSI6IlVzZXIiLCJ1c2VyIjp7ImlzX29yZ19hZG1pbiI6dHJ1ZX0sImludGVybmFsIjp7Im9yZ19pZCI6Im9yZzEyMyJ9fX0K"

sample-request-export-download:
	curl -X GET http://localhost:8000/api/export/v1/exports/$(EXPORT_ID) -H "x-rh-identity: eyJpZGVudGl0eSI6IHsiYWNjb3VudF9udW1iZXIiOiJhY2NvdW50MTIzIiwib3JnX2lkIjoib3JnMTIzIiwidHlwZSI6IlVzZXIiLCJ1c2VyIjp7ImlzX29yZ19hZG1pbiI6dHJ1ZX0sImludGVybmFsIjp7Im9yZ19pZCI6Im9yZzEyMyJ9fX0K" -f --output ./export_download.zip

sample-request-internal-upload:
	curl -X POST http://localhost:10010/app/export/v1/${EXPORT_ID}/${EXPORT_APPLICATION}/${EXPORT_RESOURCE}/upload -H "x-rh-exports-psk: testing-a-psk" -H "Content-Type: application/zip" --data-binary @example_export_upload.zip

make test:
	ginkgo -r --race --randomize-all --randomize-suites