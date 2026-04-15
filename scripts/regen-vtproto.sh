#!/usr/bin/env bash
set -euo pipefail

# Regenerates vtproto comparison code used in benchmarks.
# Requires: protoc, protoc-gen-go, protoc-gen-go-vtproto

PROTO_ROOT=$(mktemp -d)
trap 'rm -rf "$PROTO_ROOT"' EXIT

mkdir -p "$PROTO_ROOT/opentelemetry/proto/"{common/v1,resource/v1,metrics/v1,trace/v1,logs/v1,profiles/v1development}
cp proto/common.proto "$PROTO_ROOT/opentelemetry/proto/common/v1/"
cp proto/resource.proto "$PROTO_ROOT/opentelemetry/proto/resource/v1/"
cp proto/metrics.proto "$PROTO_ROOT/opentelemetry/proto/metrics/v1/"
cp proto/trace.proto "$PROTO_ROOT/opentelemetry/proto/trace/v1/"
cp proto/logs.proto "$PROTO_ROOT/opentelemetry/proto/logs/v1/"
cp proto/profiles.proto "$PROTO_ROOT/opentelemetry/proto/profiles/v1development/"

MODULE=grafana-protoc

protoc -I "$PROTO_ROOT" \
	--go_out=. --go_opt=module=$MODULE \
	--go_opt=Mopentelemetry/proto/common/v1/common.proto=$MODULE/bench/vtpb/common/v1 \
	--go_opt=Mopentelemetry/proto/resource/v1/resource.proto=$MODULE/bench/vtpb/resource/v1 \
	--go_opt=Mopentelemetry/proto/metrics/v1/metrics.proto=$MODULE/bench/vtpb/metrics/v1 \
	--go_opt=Mopentelemetry/proto/trace/v1/trace.proto=$MODULE/bench/vtpb/trace/v1 \
	--go_opt=Mopentelemetry/proto/logs/v1/logs.proto=$MODULE/bench/vtpb/logs/v1 \
	--go_opt=Mopentelemetry/proto/profiles/v1development/profiles.proto=$MODULE/bench/vtpb/profiles/v1development \
	--go-vtproto_out=. --go-vtproto_opt=module=$MODULE \
	--go-vtproto_opt=features=marshal+unmarshal+size \
	--go-vtproto_opt=Mopentelemetry/proto/common/v1/common.proto=$MODULE/bench/vtpb/common/v1 \
	--go-vtproto_opt=Mopentelemetry/proto/resource/v1/resource.proto=$MODULE/bench/vtpb/resource/v1 \
	--go-vtproto_opt=Mopentelemetry/proto/metrics/v1/metrics.proto=$MODULE/bench/vtpb/metrics/v1 \
	--go-vtproto_opt=Mopentelemetry/proto/trace/v1/trace.proto=$MODULE/bench/vtpb/trace/v1 \
	--go-vtproto_opt=Mopentelemetry/proto/logs/v1/logs.proto=$MODULE/bench/vtpb/logs/v1 \
	--go-vtproto_opt=Mopentelemetry/proto/profiles/v1development/profiles.proto=$MODULE/bench/vtpb/profiles/v1development \
	opentelemetry/proto/common/v1/common.proto \
	opentelemetry/proto/resource/v1/resource.proto \
	opentelemetry/proto/metrics/v1/metrics.proto \
	opentelemetry/proto/trace/v1/trace.proto \
	opentelemetry/proto/logs/v1/logs.proto \
	opentelemetry/proto/profiles/v1development/profiles.proto
