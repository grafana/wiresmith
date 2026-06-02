# Multi-stage Dockerfile for running protobuf conformance tests.
#
# Stage 1: Build the C++ conformance_test_runner from protobuf source.
# Stage 2: Build the Go testee binary using wiresmith-generated code.
# Stage 3: Minimal runtime image that runs the conformance tests.
#
# Usage:
#   docker build -f test/conformance/Dockerfile -t wiresmith-conformance .
#   docker run --rm wiresmith-conformance

# --- Stage 1: Build conformance_test_runner ---
FROM debian:trixie-slim@sha256:cedb1ef40439206b673ee8b33a46a03a0c9fa90bf3732f54704f99cb061d2c5a AS runner-builder

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
FROM golang:1.26-bookworm@sha256:386d475a660466863d9f8c766fec64d7fdad3edac2c6a05020c09534d71edb4b AS testee-builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
# Copy only what the testee binary needs so the cache invalidates precisely
# when dependencies change, not on unrelated file edits.
COPY gen/protohelpers/ gen/protohelpers/
COPY gen/protobuf_test_messages/ gen/protobuf_test_messages/
COPY test/conformance/internal/ test/conformance/internal/
COPY test/conformance/testee/ test/conformance/testee/
RUN CGO_ENABLED=0 go build -o /testee ./test/conformance/testee/

# --- Stage 3: Run conformance tests ---
FROM debian:trixie-slim@sha256:cedb1ef40439206b673ee8b33a46a03a0c9fa90bf3732f54704f99cb061d2c5a

RUN apt-get update && apt-get install -y --no-install-recommends \
    libjsoncpp26 \
 && rm -rf /var/lib/apt/lists/*

COPY --from=runner-builder /protobuf/conformance_test_runner /usr/local/bin/
COPY --from=testee-builder /testee /usr/local/bin/
COPY test/conformance/failure_list.txt /failure_list.txt

ENTRYPOINT ["conformance_test_runner", "--failure_list", "/failure_list.txt", "/usr/local/bin/testee"]
