# syntax=docker/dockerfile:1.7

ARG TARGETOS
ARG TARGETARCH

FROM --platform=$BUILDPLATFORM golang:1.26-alpine3.24 AS builder

WORKDIR /src

COPY --link cmd/ cmd/
COPY --link internal/ internal/
COPY --link protobuf/ protobuf/
COPY --link rpc_server/ rpc_server/
COPY --link rpc_services/ rpc_services/
COPY --link utils/ utils/
COPY --link eebus_service/ eebus_service/
COPY --link go.mod go.sum ./

RUN apk add --no-cache git && go mod download && GOBIN=/out go install github.com/grpc-ecosystem/grpc-health-probe@v0.4.52    

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 \
    go build \
        -trimpath \
        -ldflags="-s -w" \
        -o /out/eebus-grpc \
        ./cmd/main.go

FROM --platform=$TARGETPLATFORM alpine:3.24

RUN apk add --no-cache \
        ca-certificates \
        bash \
        tzdata

COPY --from=builder /out/grpc-health-probe /usr/local/bin/
COPY --from=builder /out/eebus-grpc /usr/local/bin/
COPY --chmod=755 entrypoint.sh /usr/local/bin/

RUN addgroup -S app && \
    adduser -S \
        -G app \
        -h /home/app \
        -s /sbin/bash \
        app && \
    mkdir -p /certs /app && \
    chown -R app:app /certs /app

USER app

WORKDIR /app

# GRPC Port
EXPOSE 50051

VOLUME /certs

# default value for bind address for GRPC server
ENV IPV4_ADDR="0.0.0.0"
ENV CRT_PATH="/certs/myhems_cert"
ENV KEY_PATH="/certs/myhems_key"
ENV GRPC_PORT="50051"
ENTRYPOINT ["/bin/bash", "-c", "/usr/local/bin/entrypoint.sh"]
