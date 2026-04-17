Google protobuf conformance tests for wiresmith. Runs the official `conformance_test_runner` (C++) against a Go testee that uses wiresmith-generated code for `TestAllTypesProto3`.

Everything runs in Docker (`make conformance`). The Dockerfile builds the C++ runner from source and the Go testee, then executes the test suite.

`failure_list.txt` lists expected failures. The runner flags regressions (new failures) and progress (previously-failing tests that now pass). When fixing conformance issues, remove the corresponding line from the failure list.

## Directory layout

- `../proto/conformance/conformance.proto` — Runner/testee protocol (standard protoc-gen-go)
- `../proto/conformance/test_messages_proto3.proto` — Stripped test messages (wiresmith-supported features only, original field numbers preserved)
- `testee/` — Go binary: wiresmith unmarshal/marshal, standard proto for the envelope
- `internal/conformancepb/` — Generated Go code for the conformance protocol

Both proto files are compiled via `make generate-ours`.
