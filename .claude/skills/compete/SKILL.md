---
name: compete
description: Analyze how competing protobuf libraries (gogoproto, vtprotobuf, official Go proto) implement a specific feature. Reports implementation approach, test patterns worth porting, and gaps in wiresmith's coverage.
user-invocable: true
disable-model-invocation: false
allowed-tools: Bash, Read, Grep, Glob, Agent, WebFetch, WebSearch
---

# Competitor Library Analysis

Analyze how competing protobuf libraries implement a feature and identify test patterns to port to wiresmith.

## Steps

1. Search wiresmith (`compiler/`, `gen/`, `test/`) to understand the current implementation and test coverage.

2. For each competitor, fetch implementation and tests via `raw.githubusercontent.com` URLs (use `WebSearch` to locate files if needed):
   - **gogoproto**: `gogo/protobuf` — `proto/`, `protoc-gen-gogo/generator/`
   - **vtprotobuf**: `planetscale/vtprotobuf` — `generator/`, `features/`
   - **official Go proto**: `protocolbuffers/protobuf-go` — `proto/`, `internal/impl/`, `encoding/protowire/`

3. Report: for each library, summarize approach differences and notable test edge cases (encoding corner cases, malformed input, cross-library compat). End with **recommended actions** — specific tests to add to wiresmith, with rationale. Focus on actionable gaps, not implementation suggestions unless there's a correctness issue.
