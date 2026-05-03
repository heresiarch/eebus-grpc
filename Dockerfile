# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.24.1-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

RUN apk add --no-cache ca-certificates git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/eebus-grpc ./cmd/main.go

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/create-cert ./cmd/create_cert

FROM debian:trixie-slim

WORKDIR /app

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates bash curl procps iproute2 openssl \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/eebus-grpc /usr/local/bin/eebus-grpc
COPY --from=builder /out/create-cert /usr/local/bin/create-cert

RUN mkdir -p /certs

EXPOSE 50051

ENTRYPOINT ["eebus-grpc"]
CMD ["-certificate-path", "/certs/myhems_cert", "-private-key-path", "/certs/myhems_key", "-port", "50051"]