
OS := $(shell uname)
UNAME_S := $(shell uname -s)
OS_SED :=
ifeq ($(UNAME_S),Darwin)
	OS_SED += ""
endif

OCI_TOOL=$(shell command -v podman || command -v docker)
CONTAINER_TAG="quay.io/cloudservices/export-service-go"

help:
	@echo "Please use \`make <target>' where <target> is one of:"
	@echo ""
	@echo "--- General Commands ---"
	@echo "help                     show this message"
	@echo "lint                     runs go lint on the project"
	@echo "vet                      runs go vet on the project"
	@echo "build                    builds the container image"	
	@echo ""


vet:
	go vet

lint:
	golint

build:
	$(OCI_TOOL) build . -t $(CONTAINER_TAG)