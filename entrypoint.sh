#!/bin/sh
set -e
exec /usr/local/bin/eebus-grpc \
    -certificate-path "${CRT_PATH}" \
    -private-key-path "${KEY_PATH}" \
    -port "${GRPC_PORT}" \
    -ipv4Addr "${IPV4_ADDR}" \
    "$@"

