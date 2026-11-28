################################
# STEP 1 build executable binary
################################
FROM registry.access.redhat.com/hi/go:latest-fips-builder@sha256:12999eb49f363d9e580dde98e911bd5ba3a9d1deb484d77ab8822720e32bbc97 AS builder

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
FROM registry.access.redhat.com/hi/go:latest-fips@sha256:ebba7318a970b8a5388ea4248e9e0b3d6bc78a0c68c9f0f937633948ff8cc8bc

WORKDIR /

COPY --from=builder /workspace/export-service /usr/bin
COPY --from=builder /workspace/db/migrations /db/migrations/
COPY --from=builder /workspace/static/spec/openapi.json /var/tmp/openapi.json
COPY --from=builder /workspace/static/spec/private.json /var/tmp/private.json

COPY licenses/LICENSE /licenses/LICENSE

USER 1001

CMD ["export-service"]
