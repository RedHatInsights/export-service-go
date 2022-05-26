
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

docker-up-db:
	$(DOCKER_COMPOSE) up -d db
	@until pg_isready -h $${POSTGRES_SQL_SERVICE_HOST:-localhost} -p $${POSTGRES_SQL_SERVICE_PORT:-15433} >/dev/null ; do \
		printf '.'; \
		sleep 0.5 ; \
	done

docker-up-no-server: docker-up-db
	docker-compose up -d kafka s3

monitor-topic:
	docker exec -ti kafka /usr/bin/kafka-console-consumer --bootstrap-server localhost:9092 --topic platform.export.requests
