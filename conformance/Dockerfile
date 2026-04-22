# Multi-stage Dockerfile for running protobuf conformance tests.
#
# Stage 1: Build the C++ conformance_test_runner from protobuf source.
# Stage 2: Build the Go testee binary using wiresmith-generated code.
# Stage 3: Minimal runtime image that runs the conformance tests.
#
# Usage:
#   docker build -f conformance/Dockerfile -t wiresmith-conformance .
#   docker run --rm wiresmith-conformance

# --- Stage 1: Build conformance_test_runner ---
FROM debian:trixie-slim@sha256:e02d6676e4dc2633fe045d6435d6039396736055976136f71ee5b77498f96fcf AS runner-builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    cmake make g++ git ca-certificates \
 && rm -rf /var/lib/apt/lists/*

ARG PROTOBUF_REF=v34.1
RUN git clone --depth 1 --branch ${PROTOBUF_REF} https://github.com/protocolbuffers/protobuf.git /protobuf

WORKDIR /protobuf
RUN cmake . \
      -Dprotobuf_BUILD_CONFORMANCE=ON \
      -Dprotobuf_BUILD_TESTS=OFF \
      -DCMAKE_BUILD_TYPE=Release \
 && cmake --build . --target conformance_test_runner -- -j"$(nproc)"

# --- Stage 2: Build Go testee ---
FROM golang:1.26@sha256:46d487a9216d9d3563ae7be4ee0f6a4aa9a3f6befdf62c384fd5118a7e254c4d AS testee-builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
# Copy only what the testee binary needs so the cache invalidates precisely
# when dependencies change, not on unrelated file edits.
COPY gen/protohelpers/ gen/protohelpers/
COPY gen/protobuf_test_messages/ gen/protobuf_test_messages/
COPY conformance/internal/ conformance/internal/
COPY conformance/testee/ conformance/testee/
RUN CGO_ENABLED=0 go build -o /testee ./conformance/testee/

# --- Stage 3: Run conformance tests ---
FROM debian:trixie-slim@sha256:e02d6676e4dc2633fe045d6435d6039396736055976136f71ee5b77498f96fcf

RUN apt-get update && apt-get install -y --no-install-recommends \
    libjsoncpp26 \
 && rm -rf /var/lib/apt/lists/*

COPY --from=runner-builder /protobuf/conformance_test_runner /usr/local/bin/
COPY --from=testee-builder /testee /usr/local/bin/
COPY conformance/failure_list.txt /failure_list.txt

ENTRYPOINT ["conformance_test_runner", "--failure_list", "/failure_list.txt", "/usr/local/bin/testee"]
