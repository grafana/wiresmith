#!/usr/bin/env bash
set -euo pipefail

# Generates all protobuf code: grafana-protoc, vtproto, and gogoproto.
# Requires: protoc, protoc-gen-go, protoc-gen-go-vtproto, protoc-gen-gogofast

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

MODULE=grafana-protoc

SIGNALS=(common/v1 resource/v1 metrics/v1 trace/v1 logs/v1 profiles/v1development)
PROTO_FILES=()
for s in "${SIGNALS[@]}"; do
	PROTO_FILES+=("opentelemetry/proto/$s/$(basename "$s" | sed 's/v1development/profiles/;s/v1//')$(basename "$s" | grep -q development && echo '' || echo '').proto")
done

# Build the canonical proto directory layout that matches import paths.
PROTO_ROOT=$(mktemp -d)
trap 'rm -rf "$PROTO_ROOT"' EXIT

mkdir -p "$PROTO_ROOT/opentelemetry/proto/"{common/v1,resource/v1,metrics/v1,trace/v1,logs/v1,profiles/v1development}
cp "$PROJECT_ROOT/proto/common.proto" "$PROTO_ROOT/opentelemetry/proto/common/v1/"
cp "$PROJECT_ROOT/proto/resource.proto" "$PROTO_ROOT/opentelemetry/proto/resource/v1/"
cp "$PROJECT_ROOT/proto/metrics.proto" "$PROTO_ROOT/opentelemetry/proto/metrics/v1/"
cp "$PROJECT_ROOT/proto/trace.proto" "$PROTO_ROOT/opentelemetry/proto/trace/v1/"
cp "$PROJECT_ROOT/proto/logs.proto" "$PROTO_ROOT/opentelemetry/proto/logs/v1/"
cp "$PROJECT_ROOT/proto/profiles.proto" "$PROTO_ROOT/opentelemetry/proto/profiles/v1development/"

ALL_PROTOS=(
	opentelemetry/proto/common/v1/common.proto
	opentelemetry/proto/resource/v1/resource.proto
	opentelemetry/proto/metrics/v1/metrics.proto
	opentelemetry/proto/trace/v1/trace.proto
	opentelemetry/proto/logs/v1/logs.proto
	opentelemetry/proto/profiles/v1development/profiles.proto
)

# Map function: generates --go_opt=M... flags for a given output prefix.
mflags() {
	local prefix=$1 optname=$2
	for p in "${ALL_PROTOS[@]}"; do
		# opentelemetry/proto/trace/v1/trace.proto → trace/v1
		local rel="${p#opentelemetry/proto/}"
		local dir="${rel%/*}"
		echo "--${optname}=M${p}=${MODULE}/${prefix}/${dir}"
	done
}

# ── Step 1: grafana-protoc (our code) ──────────────────────────────────────────
echo "==> Generating grafana-protoc code → gen/otlp/"
(cd "$PROJECT_ROOT" && go run ./cmd/grafana-protoc/ --proto_path=proto --out=gen --module="$MODULE")

# ── Step 2: vtproto ───────────────────────────────────────────────────────────
echo "==> Generating vtproto code → gen/vtpb/"
(cd "$PROJECT_ROOT" && protoc -I "$PROTO_ROOT" \
	--go_out=. --go_opt=module=$MODULE \
	$(mflags gen/vtpb go_opt) \
	--go-vtproto_out=. --go-vtproto_opt=module=$MODULE \
	--go-vtproto_opt=features=marshal+unmarshal+size \
	$(mflags gen/vtpb go-vtproto_opt) \
	"${ALL_PROTOS[@]}")

# ── Step 3: gogoproto ────────────────────────────────────────────────────────
echo "==> Generating gogoproto code → gen/gogopb/"

# gogofast predates proto3 optional keyword — strip it from a temp copy.
GOGO_ROOT=$(mktemp -d)
trap 'rm -rf "$PROTO_ROOT" "$GOGO_ROOT"' EXIT

cp -r "$PROTO_ROOT/opentelemetry" "$GOGO_ROOT/"
# Remove 'optional' keyword from proto3 fields (e.g. "optional double sum = 5;")
find "$GOGO_ROOT" -name '*.proto' -exec sed -i '' 's/^  optional /  /g' {} +
# Rewrite go_package to target gen/gogopb/ so gogofast outputs to the right directory.
for p in "${ALL_PROTOS[@]}"; do
	rel="${p#opentelemetry/proto/}"
	dir="${rel%/*}"
	sed -i '' "s|option go_package = .*|option go_package = \"${MODULE}/gen/gogopb/${dir}\";|" "$GOGO_ROOT/$p"
done

# gogoproto uses comma-separated M-flags in --gogofast_out=<opts>:<dir>
# Must generate one proto at a time because gogofast can't handle multiple packages.
GOGO_MFLAGS=""
for p in "${ALL_PROTOS[@]}"; do
	rel="${p#opentelemetry/proto/}"
	dir="${rel%/*}"
	GOGO_MFLAGS="${GOGO_MFLAGS}M${p}=${MODULE}/gen/gogopb/${dir},"
done

# gogofast writes to <out>/<full_go_package_path>. We use a temp output dir
# then move files into the project, avoiding dependency on the project dirname.
GOGO_OUT=$(mktemp -d)
trap 'rm -rf "$PROTO_ROOT" "$GOGO_ROOT" "$GOGO_OUT"' EXIT

for p in "${ALL_PROTOS[@]}"; do
	protoc -I "$GOGO_ROOT" \
		--gogofast_out="${GOGO_MFLAGS%,}:$GOGO_OUT" \
		"$p"
done

rm -rf "$PROJECT_ROOT/gen/gogopb"
mv "$GOGO_OUT/$MODULE/gen/gogopb" "$PROJECT_ROOT/gen/gogopb"

echo "==> Done."
