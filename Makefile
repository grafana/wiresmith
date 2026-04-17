.DEFAULT_GOAL := help

MODULE := wiresmith

ALL_PROTOS := \
	opentelemetry/proto/common/v1/common.proto \
	opentelemetry/proto/resource/v1/resource.proto \
	opentelemetry/proto/metrics/v1/metrics.proto \
	opentelemetry/proto/trace/v1/trace.proto \
	opentelemetry/proto/logs/v1/logs.proto \
	opentelemetry/proto/profiles/v1development/profiles.proto

PROTO_DIRS := common/v1 resource/v1 metrics/v1 trace/v1 logs/v1 profiles/v1development

# Map proto import paths to Go packages for a given output prefix.
# Usage: $(call mflags,gen/vtpb,go_opt)
define mflags
$(foreach p,$(ALL_PROTOS),--$(2)=M$(p)=$(MODULE)/$(1)/$(patsubst opentelemetry/proto/%,%,$(dir $(p))))
endef

.PHONY: help build test fuzz generate bench bench-compare clean conformance generate-conformance

help: ## Print this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'

build: ## Build all packages
	go build ./...

test: ## Run correctness tests
	go test ./test/ -v

fuzz: ## Fuzz all targets (30s each)
	@for target in FuzzUnmarshal FuzzRoundTrip FuzzMarshalSize FuzzCrossLibrary FuzzStructuredTrace FuzzStructuredMetrics FuzzStructuredLogs; do \
		echo "==> Fuzzing $$target..."; \
		GOLANG_PROTOBUF_REGISTRATION_CONFLICT=warn \
			go test ./test/ -fuzz $$target -fuzztime 30s -run='^$$' || exit 1; \
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
	docker build -f conformance/Dockerfile -t wiresmith-conformance .
	docker run --rm wiresmith-conformance

clean: ## Remove all generated code under gen/
	rm -rf gen/otlp gen/vtpb gen/gogopb gen/protobuf_test_messages

# ── Generate targets ─────────────────────────────────────────────────────────

.PHONY: generate-ours generate-vtproto generate-gogoproto proto-root

# Build the canonical proto directory layout that matches import paths.
# Sets PROTO_ROOT as a temp directory and copies proto files into it.
define setup_proto_root
	$(eval PROTO_ROOT := $(shell mktemp -d))
	@mkdir -p $(foreach d,$(PROTO_DIRS),"$(PROTO_ROOT)/opentelemetry/proto/$(d)")
	@cp proto/common.proto   "$(PROTO_ROOT)/opentelemetry/proto/common/v1/"
	@cp proto/resource.proto "$(PROTO_ROOT)/opentelemetry/proto/resource/v1/"
	@cp proto/metrics.proto  "$(PROTO_ROOT)/opentelemetry/proto/metrics/v1/"
	@cp proto/trace.proto    "$(PROTO_ROOT)/opentelemetry/proto/trace/v1/"
	@cp proto/logs.proto     "$(PROTO_ROOT)/opentelemetry/proto/logs/v1/"
	@cp proto/profiles.proto "$(PROTO_ROOT)/opentelemetry/proto/profiles/v1development/"
endef

generate-conformance: ## Regenerate conformance test code
	@echo "==> Generating conformance protocol code → conformance/internal/conformancepb/"
	protoc -I conformance/proto \
		--go_out=. --go_opt=module=$(MODULE) \
		conformance/proto/conformance.proto
	@echo "==> Generating wiresmith code for test messages → gen/protobuf_test_messages/"
	go run ./cmd/wiresmith/ --proto_path=conformance/testmsg --out=gen --module=$(MODULE)

generate-ours:
	@echo "==> Generating wiresmith code → gen/otlp/"
	go run ./cmd/wiresmith/ --proto_path=proto --out=gen --module=$(MODULE)
	@echo "==> Generating wiresmith code → gen/test/kitchensink/"
	go run ./cmd/wiresmith/ --proto_path=test/testdata --out=gen --module=$(MODULE)

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
		sed -i '' 's|option go_package = .*|option go_package = "$(MODULE)/gen/gogopb/$(patsubst opentelemetry/proto/%,%,$(dir $(p)))";|' \
			"$(GOGO_ROOT)/$(p)";)
	@$(foreach p,$(ALL_PROTOS),\
		protoc -I "$(GOGO_ROOT)" \
			--gogofast_out=$(foreach gp,$(ALL_PROTOS),M$(gp)=$(MODULE)/gen/gogopb/$(patsubst opentelemetry/proto/%,%,$(dir $(gp)))$(comma)):"$(GOGO_OUT)" \
			$(p);)
	@rm -rf gen/gogopb
	@mv "$(GOGO_OUT)/$(MODULE)/gen/gogopb" gen/gogopb
	@rm -rf "$(PROTO_ROOT)" "$(GOGO_ROOT)" "$(GOGO_OUT)"

# Needed for comma in $(foreach) expansions.
comma := ,
