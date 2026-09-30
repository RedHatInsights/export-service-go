################################
# STEP 1 build executable binary
################################
FROM registry.access.redhat.com/ubi9/go-toolset:latest@sha256:0a4666f7a4eb0644c97a73cba198eb268691b270d97831822689e7a2088f87be AS builder

USER root

WORKDIR /workspace
# Cache deps before copying source so that we do not need to re-download for every build
COPY go.mod go.sum .

# Fetch dependencies
RUN go mod download

# -x flag for more verbose download logging
# RUN go mod download -x

# Now copy the rest of the files for build
COPY docs docs
COPY s3 s3
COPY metrics metrics
COPY cmd cmd
COPY static static
COPY db db
COPY utils utils
COPY config config
COPY logger logger
COPY exports exports
COPY kafka kafka
COPY models models
COPY middleware middleware

# Build the binary
RUN GO111MODULE=on go build -ldflags "-w -s" -o export-service cmd/export-service/*.go
############################
# STEP 2 build a small image
############################
FROM registry.access.redhat.com/ubi9-minimal:latest@sha256:beeada7dd17903dfb69fd5f6916c054720bf28a52daaa2f7a1910a1394244bd2

RUN microdnf update -y

COPY --from=builder /workspace/export-service /usr/bin
COPY --from=builder /workspace/db/migrations /db/migrations/
COPY --from=builder /workspace/static/spec/openapi.json /var/tmp/openapi.json
COPY --from=builder /workspace/static/spec/private.json /var/tmp/private.json

COPY licenses/LICENSE /licenses/LICENSE

USER 1001

CMD ["export-service"]
