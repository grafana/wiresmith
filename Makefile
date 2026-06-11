.DEFAULT_GOAL := help

MODULE := github.com/grafana/wiresmith

ALL_PROTOS := \
	opentelemetry/proto/common/v1/common.proto \
	opentelemetry/proto/resource/v1/resource.proto \
	opentelemetry/proto/metrics/v1/metrics.proto \
	opentelemetry/proto/trace/v1/trace.proto \
	opentelemetry/proto/logs/v1/logs.proto \
	opentelemetry/proto/profiles/v1development/profiles.proto

PROTO_DIRS := common/v1 resource/v1 metrics/v1 trace/v1 logs/v1 profiles/v1development

# Go package suffix for a proto path: opentelemetry/proto/common/v1/common.proto → common/v1
pkgsuffix = $(patsubst %/,%,$(patsubst opentelemetry/proto/%,%,$(dir $(1))))

# Map proto import paths to Go packages for a given output prefix.
# Usage: $(call mflags,gen/vtpb,go_opt)
# Produces: --go_opt=Mcommon.proto=github.com/grafana/wiresmith/gen/vtpb/common/v1 --go_opt=M...
define mflags
$(foreach p,$(ALL_PROTOS),--$(2)=M$(p)=$(MODULE)/$(1)/$(call pkgsuffix,$(p)))
endef

# wiresmith's -M flag (single hyphen, takes one src=dest per occurrence)
# overrides each OTel proto's upstream `go_package = "go.opentelemetry.io/..."`
# so generated imports match the on-disk source-relative output. Matches
# protoc-gen-go's M<src>=<dest> semantics; the vtproto/gogoproto passes
# above use the same trick at the protoc level.
#
# The `;name` suffix pins the Go package clause to wiresmith's historical
# synthetic name (commonv1, logsv1, …) instead of path.Base's "v1". This
# preserves the existing API of gen/opentelemetry/proto/...; switching to
# the path.Base default (matching vtproto/gogoproto) is deferred — see
# the rename bead linked from FLAGS.md.
define wiresmith_mflags
$(foreach p,$(ALL_PROTOS),-M $(p)=$(MODULE)/gen/opentelemetry/proto/$(call pkgsuffix,$(p))\;$(subst /,,$(call pkgsuffix,$(p))))
endef

# Comma-separated M-flags for gogoproto (no spaces — $(foreach) joins with spaces
# so we strip them to produce the single token gogofast_out= expects).
empty :=
space := $(empty) $(empty)
comma := ,
gogo_mflags = $(subst $(space),,$(foreach p,$(ALL_PROTOS),M$(p)=$(MODULE)/$(1)/$(call pkgsuffix,$(p))$(comma)))

.PHONY: help build test test-race coverage fuzz generate bench bench-compare clean conformance

help: ## Print this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'

build: ## Build all packages
	go build ./...

test: ## Run correctness tests
	GOLANG_PROTOBUF_REGISTRATION_CONFLICT=warn go test ./test/... ./cmd/... ./compiler/... -v

test-race: ## Run the race detector over concurrency-sensitive tests
	GOLANG_PROTOBUF_REGISTRATION_CONFLICT=warn go test -race ./test/peer/ ./test/basic/ ./test/differential/

coverage: ## Run tests with coverage report
	GOLANG_PROTOBUF_REGISTRATION_CONFLICT=warn go test ./test/... ./compiler/... -coverpkg=./compiler/...,./protohelpers/... -coverprofile=coverage.out
	go tool cover -func=coverage.out
	@echo ""
	@echo "HTML report: go tool cover -html=coverage.out"

fuzz: ## Fuzz all targets (30s each) — auto-discovers Fuzz* functions in ./test/fuzz/
	@targets=$$(GOLANG_PROTOBUF_REGISTRATION_CONFLICT=warn \
		go test ./test/fuzz/ -list '^Fuzz' | grep '^Fuzz'); \
	if [ -z "$$targets" ]; then \
		echo "No fuzz targets found in ./test/fuzz/"; exit 1; \
	fi; \
	for target in $$targets; do \
		echo "==> Fuzzing $$target..."; \
		GOLANG_PROTOBUF_REGISTRATION_CONFLICT=warn \
			go test ./test/fuzz/ -fuzz "^$$target$$" -fuzztime 30s -run='^$$' || exit 1; \
	done

generate: generate-ours generate-vtproto generate-gogoproto ## Regenerate all code (ours + vtproto + gogoproto)

bench: ## Run comparative benchmarks
	GOLANG_PROTOBUF_REGISTRATION_CONFLICT=warn go test ./bench/ -bench=. -benchmem -count=5

COUNT ?= -count=10
bench-compare: ## Run per-library benchmarks and compare with benchstat
	@command -v benchstat >/dev/null 2>&1 || { echo "benchstat not found. Install with: go install golang.org/x/perf/cmd/benchstat@latest"; exit 1; }
	$(eval TMPDIR := $(shell mktemp -d))
	@trap 'rm -rf "$(TMPDIR)"' EXIT; \
	for lib in Ours Official VTProto GogoProto; do \
		echo "==> Running benchmarks for $$lib..."; \
		GOLANG_PROTOBUF_REGISTRATION_CONFLICT=warn \
			go test ./bench/ -bench=".*_$${lib}$$" -benchmem $(COUNT) -run='^$$' | \
			sed "s/_$${lib}-/-/" \
				>"$(TMPDIR)/$${lib}.txt"; \
	done; \
	echo ""; \
	echo "==> Comparing with benchstat (Ours vs Official vs VTProto vs GogoProto):"; \
	echo ""; \
	benchstat \
		"Ours=$(TMPDIR)/Ours.txt" \
		"Official=$(TMPDIR)/Official.txt" \
		"VTProto=$(TMPDIR)/VTProto.txt" \
		"GogoProto=$(TMPDIR)/GogoProto.txt"

conformance: ## Run conformance tests (requires Docker)
	docker build -f test/conformance/Dockerfile -t wiresmith-conformance .
	docker run --rm wiresmith-conformance

clean: ## Remove all generated code under gen/ (protohelpers/ at repo root is checked-in source)
	@find gen -mindepth 1 -maxdepth 1 -type d -exec rm -rf {} +

# ── Generate targets ─────────────────────────────────────────────────────────

.PHONY: generate-ours generate-vtproto generate-gogoproto

# Build the canonical proto directory layout that matches import paths.
# Sets PROTO_ROOT as a temp directory and copies proto files into it.
define setup_proto_root
	$(eval PROTO_ROOT := $(shell mktemp -d))
	@mkdir -p $(foreach d,$(PROTO_DIRS),"$(PROTO_ROOT)/opentelemetry/proto/$(d)")
	@cp proto/otlp/common.proto   "$(PROTO_ROOT)/opentelemetry/proto/common/v1/"
	@cp proto/otlp/resource.proto "$(PROTO_ROOT)/opentelemetry/proto/resource/v1/"
	@cp proto/otlp/metrics.proto  "$(PROTO_ROOT)/opentelemetry/proto/metrics/v1/"
	@cp proto/otlp/trace.proto    "$(PROTO_ROOT)/opentelemetry/proto/trace/v1/"
	@cp proto/otlp/logs.proto     "$(PROTO_ROOT)/opentelemetry/proto/logs/v1/"
	@cp proto/otlp/profiles.proto "$(PROTO_ROOT)/opentelemetry/proto/profiles/v1development/"
endef

generate-ours: ## Regenerate all wiresmith + conformance code
	$(eval WIRESMITH := $(shell go build -o /tmp/wiresmith-gen ./cmd/wiresmith/ && echo /tmp/wiresmith-gen))
	@echo "==> Generating wiresmith code → gen/opentelemetry/proto/"
	$(WIRESMITH) --proto_path=proto/otlp --out=gen --module=$(MODULE) $(call wiresmith_mflags)
	@echo "==> Generating wiresmith code → gen/test/kitchensink/"
	$(WIRESMITH) --proto_path=proto/test --out=gen --module=$(MODULE)
	@echo "==> Generating wiresmith code → gen/basic/"
	$(WIRESMITH) --proto_path=proto/basic --out=gen --module=$(MODULE)
	@# gen/basic/service/v1/service_grpc.pb.go is emitted by the wiresmith
	@# CLI above via the vendored protoc-gen-go-grpc v1.6.0 generator (see
	@# compiler/generator/grpc/). No separate protoc-gen-go-grpc invocation
	@# is needed — adopters no longer have to install the standalone plugin.
	@echo "==> Generating wiresmith conformance test messages → gen/protobuf_test_messages/"
	$(WIRESMITH) --proto_path=proto/conformance --out=gen --module=$(MODULE) proto/conformance/test_messages_proto3.proto
	@echo "==> Generating conformance protocol code → test/conformance/internal/conformancepb/"
	protoc -I proto/conformance \
		--go_out=. --go_opt=module=$(MODULE) \
		proto/conformance/conformance.proto
	@echo "==> Generating official proto bench code → gen/bench/official/"
	protoc -I proto/basic \
		--go_out=. --go_opt=module=$(MODULE) \
		--go_opt=Mmaps.proto=github.com/grafana/wiresmith/gen/bench/official \
		proto/basic/maps.proto

generate-vtproto:
	$(setup_proto_root)
	@echo "==> Generating vtproto code → gen/vtpb/"
	protoc -I "$(PROTO_ROOT)" \
		--go_out=. --go_opt=module=$(MODULE) \
		$(call mflags,gen/vtpb,go_opt) \
		--go-vtproto_out=. --go-vtproto_opt=module=$(MODULE) \
		--go-vtproto_opt=features=marshal+unmarshal+size \
		$(call mflags,gen/vtpb,go-vtproto_opt) \
		$(ALL_PROTOS)
	@rm -rf "$(PROTO_ROOT)"
	@echo "==> Generating vtproto bench code → gen/bench/vtpb/"
	protoc -I proto/basic \
		--go_out=. --go_opt=module=$(MODULE) \
		--go_opt=Mmaps.proto=github.com/grafana/wiresmith/gen/bench/vtpb \
		--go-vtproto_out=. --go-vtproto_opt=module=$(MODULE) \
		--go-vtproto_opt=features=marshal+unmarshal+size \
		--go-vtproto_opt=Mmaps.proto=github.com/grafana/wiresmith/gen/bench/vtpb \
		proto/basic/maps.proto

generate-gogoproto:
	$(setup_proto_root)
	$(eval GOGO_ROOT := $(shell mktemp -d))
	$(eval GOGO_OUT := $(shell mktemp -d))
	@echo "==> Generating gogoproto code → gen/gogopb/"
	@cp -r "$(PROTO_ROOT)/opentelemetry" "$(GOGO_ROOT)/"
	@# Remove 'optional' keyword — gogofast predates proto3 optional.
	@find "$(GOGO_ROOT)" -name '*.proto' -exec sed -i '' 's/^  optional /  /g' {} +
	@# Rewrite go_package to target gen/gogopb/.
	@$(foreach p,$(ALL_PROTOS),\
		sed -i '' 's|option go_package = .*|option go_package = "$(MODULE)/gen/gogopb/$(call pkgsuffix,$(p))";|' \
			"$(GOGO_ROOT)/$(p)";)
	@$(foreach p,$(ALL_PROTOS),\
		protoc -I "$(GOGO_ROOT)" \
			--gogofast_out=$(call gogo_mflags,gen/gogopb):"$(GOGO_OUT)" \
			$(p);)
	@rm -rf gen/gogopb
	@mv "$(GOGO_OUT)/$(MODULE)/gen/gogopb" gen/gogopb
	@echo "==> Generating gogoproto bench code → gen/bench/gogopb/"
	@cp proto/basic/maps.proto "$(GOGO_ROOT)/maps.proto"
	@sed -i '' 's|option go_package = .*|option go_package = "$(MODULE)/gen/bench/gogopb";|' "$(GOGO_ROOT)/maps.proto"
	@protoc -I "$(GOGO_ROOT)" \
		--gogofast_out=Mmaps.proto=$(MODULE)/gen/bench/gogopb$(comma):"$(GOGO_OUT)" \
		maps.proto
	@rm -rf gen/bench/gogopb
	@mv "$(GOGO_OUT)/$(MODULE)/gen/bench/gogopb" gen/bench/gogopb
	@rm -rf "$(PROTO_ROOT)" "$(GOGO_ROOT)" "$(GOGO_OUT)"
