---
name: validate
description: Run full pre-push validation — regenerate code, check freshness, run tests, and run conformance. Use before pushing any changes to protobuf codegen or marshal/unmarshal code.
user-invocable: true
disable-model-invocation: false
allowed-tools: Bash, Read, Grep, Glob
---

# Pre-Push Validation

Run these steps sequentially. Stop on first failure. Show full output for every step — the whole point is to see evidence, not summaries.

1. `make generate-ours`
2. `git diff --stat --exit-code gen/` — if stale, show the diff and stage changes, but continue
3. `go test ./test/ ./compiler/... -v`
4. `make conformance` — compare counts against `conformance/failure_list.txt`
