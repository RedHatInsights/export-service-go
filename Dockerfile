################################
# STEP 1 build executable binary
################################
FROM registry.access.redhat.com/hi/go:latest-fips-builder@sha256:c52ae1e6fe4d3a0bbe3b4457eca4691f762c7f71605423a4069b1c3dbd7daccc AS builder

USER 0

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
COPY securitylog securitylog

# Build the binary
RUN GO111MODULE=on go build -ldflags "-w -s" -o export-service cmd/export-service/*.go
############################
# STEP 2 build a small image
############################
FROM registry.access.redhat.com/hi/go:latest-fips@sha256:ee1ec2fd8d76564b1d45ba348508db163b2d9d3bb9a9cec166b112ab87c2bbd8

WORKDIR /

COPY --from=builder /workspace/export-service /usr/bin
COPY --from=builder /workspace/db/migrations /db/migrations/
COPY --from=builder /workspace/static/spec/openapi.json /var/tmp/openapi.json
COPY --from=builder /workspace/static/spec/private.json /var/tmp/private.json

COPY licenses/LICENSE /licenses/LICENSE

USER 1001

CMD ["export-service"]
