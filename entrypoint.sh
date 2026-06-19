#!/bin/sh
set -e
exec eebus-grpc "$@" -ipv4Addr "$IPV4_ADDR"
