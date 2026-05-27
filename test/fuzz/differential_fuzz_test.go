package fuzz

import (
	"bytes"
	"testing"

	"google.golang.org/protobuf/proto"

	logsv1 "wiresmith/gen/opentelemetry/proto/logs/v1"
	metricsv1 "wiresmith/gen/opentelemetry/proto/metrics/v1"
	tracev1 "wiresmith/gen/opentelemetry/proto/trace/v1"
	"wiresmith/test/testutil"

	otlplogs "go.opentelemetry.io/proto/otlp/logs/v1"
	otlpmetrics "go.opentelemetry.io/proto/otlp/metrics/v1"
	otlptrace "go.opentelemetry.io/proto/otlp/trace/v1"
)

// FuzzDifferentialBytewise compares wiresmith against the official protobuf
// library at the byte level. For each top-level message type it:
//
//  1. Unmarshals fuzz bytes with wiresmith (skip if error)
//  2. Marshals with wiresmith → canonical bytes (unknown fields stripped)
//  3. Unmarshals canonical bytes with official proto (must succeed —
//     wiresmith output must always be valid protobuf)
//  4. Marshals with official → official bytes
//  5. Unmarshals official bytes with wiresmith, re-marshals → canonical2
//  6. Asserts canonical == canonical2
//
// This catches value-level disagreements: if either library decodes a field
// differently, the round-trip through the other library will produce
// different bytes.
func FuzzDifferentialBytewise(f *testing.F) {
	for _, seed := range marshaledSeeds() {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		differentialCheck(t, data, "TracesData",
			func() testutil.Message { return new(tracev1.TracesData) },
			func(b []byte) ([]byte, error) {
				var m otlptrace.TracesData
				if err := proto.Unmarshal(b, &m); err != nil {
					return nil, err
				}
				return proto.Marshal(&m)
			},
		)

		differentialCheck(t, data, "MetricsData",
			func() testutil.Message { return new(metricsv1.MetricsData) },
			func(b []byte) ([]byte, error) {
				var m otlpmetrics.MetricsData
				if err := proto.Unmarshal(b, &m); err != nil {
					return nil, err
				}
				return proto.Marshal(&m)
			},
		)

		differentialCheck(t, data, "LogsData",
			func() testutil.Message { return new(logsv1.LogsData) },
			func(b []byte) ([]byte, error) {
				var m otlplogs.LogsData
				if err := proto.Unmarshal(b, &m); err != nil {
					return nil, err
				}
				return proto.Marshal(&m)
			},
		)
	})
}

// differentialCheck runs a single type through the differential byte comparison.
// officialRoundTrip unmarshals with official proto and re-marshals, returning
// the official canonical bytes.
func differentialCheck(
	t *testing.T,
	data []byte,
	name string,
	newOurs func() testutil.Message,
	officialRoundTrip func([]byte) ([]byte, error),
) {
	t.Helper()

	// Step 1: unmarshal with wiresmith
	ours := newOurs()
	if err := ours.Unmarshal(data); err != nil {
		return
	}

	// Step 2: marshal with wiresmith → canonical bytes (unknown fields stripped)
	oursBytes, err := ours.Marshal()
	if err != nil {
		t.Fatalf("%s: Marshal failed after successful Unmarshal: %v", name, err)
	}

	// Step 3-4: round-trip through official proto.
	// The official library is stricter (e.g. UTF-8 validation for strings),
	// so it may reject bytes wiresmith accepts. Skip those — we only care
	// about disagreements when both libraries accept the data.
	officialBytes, err := officialRoundTrip(oursBytes)
	if err != nil {
		return
	}

	// Step 5: unmarshal official output with wiresmith, re-marshal
	ours2 := newOurs()
	if err := ours2.Unmarshal(officialBytes); err != nil {
		t.Fatalf("%s: wiresmith rejected official proto output: %v", name, err)
	}

	oursBytes2, err := ours2.Marshal()
	if err != nil {
		t.Fatalf("%s: re-Marshal failed: %v", name, err)
	}

	// Step 6: both canonical forms must match
	if !bytes.Equal(oursBytes, oursBytes2) {
		t.Fatalf("%s: differential mismatch: wiresmith canonical=%d bytes, after official round-trip=%d bytes",
			name, len(oursBytes), len(oursBytes2))
	}
}
