# syntax=docker/dockerfile:1.7

FROM --platform=$BUILDPLATFORM debian:trixie-slim AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        golang \
        git \
        ca-certificates && \
    rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/root/go/pkg/mod \
    go mod download

COPY . .

RUN --mount=type=cache,target=/root/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/eebus-grpc ./cmd/main.go

RUN GOBIN=/out go install github.com/grpc-ecosystem/grpc-health-probe@latest    

FROM debian:trixie-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        ca-certificates procps && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/grpc-health-probe /usr/local/bin/

RUN groupadd --system eebus && \
    useradd --system \
        --gid eebus \
        --home-dir /nonexistent \
        --shell /bin/bash \
        eebus

COPY --from=builder /out/eebus-grpc /usr/local/bin/
COPY entrypoint.sh /usr/local/bin/entrypoint.sh

RUN groupadd --system app && \
    useradd --system \
            --gid app \
            --create-home \
            --home-dir /home/app \
            --shell /usr/sbin/nologin \
            app

RUN mkdir -p /certs && \
    chown app:app /certs

USER app

WORKDIR /app

# GRPC Port
EXPOSE 50051

VOLUME /certs
# default value for bind address for GRPC server
ENV IPV4_ADDR="0.0.0.0"

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["-certificate-path", "/certs/myhems_cert", "-private-key-path", "/certs/myhems_key", "-port", "50051" ]
