# EEBUS GRPC

This repository contains the EEBUS GRPC api. It is based on the `enbility/eebus-go` library.
It works as a bridge between a generic EEBUS go application and application code in other languages.
It is designed to be started/stoped and configured via gRPC calls.

## Start

```bash
go run cmd/main.go \
  -certificate-path <path_to_certificate> \
  -private-key-path <path_to_private_key> \
  -port <rpc-port>
```

To start the server, you need to provide the path to the certificate and the private key. The server will listen on the specified port.

## Structure

### eebus_service

Implements the `eebus-go/service` interface.

### internal

Contains the generation instructions of the gRPC go glue code generated with `protoc`.

The generation can be done with the following command:

```bash
go generate ./...
```

The generated code is checked in to the repository. So you don't need to run the generation command unless you change the `.proto` files.

### protobuf

Contains the `.proto` files that define the gRPC API. Which is as analogous as possible to the `eebus-go` api.

### rpc_server

Implements the gRPC server interfaces. There is the main server `control_service` that is responsible for starting, stopping and configuring the `eebus_service`.
Additionally, there is one server for each combinitation of eebus use case and actor.

### rpc_services

Contains, the generated gRPC glue code. The code is generated with the `protoc` command.

### utils

Contains utility functions.

## Helper Commands

- [Create Certificate Script](cmd/create_cert/README.md)
- [Dummy CEM](cmd/dummy_cem/README.md)


## Build image

```
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t dein-registry/eebus-grpc:latest \
  --push .
```

local testing:

```
docker buildx build --platform linux/amd64 -t eebus-grpc:amd64 --load .
docker buildx build --platform linux/arm64 -t eebus-grpc:arm64 --load .
```

## Run commands

Genertate cers:

```
docker run --rm -it \
  -v "$PWD/cert:/certs" \
  --entrypoint create-cert \
  eebus-grpc /certs myhems
```

Run grpc server

```
docker run --rm -it \
  -p 50051:50051 \
  -v "$PWD/cert:/certs" \
  eebus-grpc \
  -certificate-path /certs/myhems_cert \
  -private-key-path /certs/myhems_key \
  -port 50051
```
