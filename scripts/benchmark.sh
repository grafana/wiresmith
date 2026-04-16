#!/usr/bin/env bash
set -euo pipefail

# Runs benchmarks for each library separately and compares them with benchstat.
# Output files are written to a temp directory and cleaned up on exit.
#
# Usage:
#   ./scripts/benchmark.sh              # default 5 iterations
#   ./scripts/benchmark.sh -count=10    # custom iteration count

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

COUNT="${1:--count=5}"

if ! command -v benchstat &>/dev/null; then
	echo "benchstat not found. Install with: go install golang.org/x/perf/cmd/benchstat@latest"
	exit 1
fi

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

LIBS=("Ours" "Official" "VTProto" "GogoProto")

for lib in "${LIBS[@]}"; do
	echo "==> Running benchmarks for $lib..."
	GOLANG_PROTOBUF_REGISTRATION_CONFLICT=warn \
		go test "$PROJECT_ROOT/bench/" -bench=".*_${lib}\$" -benchmem "$COUNT" -run='^$' |
		sed "s/_${lib}-/-/" \
			>"$TMPDIR/${lib}.txt"
done

echo ""
echo "==> Comparing with benchstat (Ours vs Official vs VTProto vs GogoProto):"
echo ""

benchstat \
	"Ours=$TMPDIR/Ours.txt" \
	"Official=$TMPDIR/Official.txt" \
	"VTProto=$TMPDIR/VTProto.txt" \
	"GogoProto=$TMPDIR/GogoProto.txt"
