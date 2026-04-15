package test

import (
	"testing"

	commonv1 "grafana-protoc/gen/otlp/common/v1"
	logsv1 "grafana-protoc/gen/otlp/logs/v1"
	metricsv1 "grafana-protoc/gen/otlp/metrics/v1"
	profilesv1 "grafana-protoc/gen/otlp/profiles/v1development"
	resourcev1 "grafana-protoc/gen/otlp/resource/v1"
	tracev1 "grafana-protoc/gen/otlp/trace/v1"
)

// unmarshaler is implemented by all generated message types.
type unmarshaler interface {
	Unmarshal([]byte) error
}

// allMessageConstructors returns a constructor for every generated message type.
// Each fuzz iteration calls every constructor so that a single corpus entry
// exercises all unmarshal paths.
func allMessageConstructors() map[string]func() unmarshaler {
	return map[string]func() unmarshaler{
		// common/v1
		"AnyValue":             func() unmarshaler { return new(commonv1.AnyValue) },
		"ArrayValue":           func() unmarshaler { return new(commonv1.ArrayValue) },
		"InstrumentationScope": func() unmarshaler { return new(commonv1.InstrumentationScope) },
		"KeyValue":             func() unmarshaler { return new(commonv1.KeyValue) },
		"KeyValueList":         func() unmarshaler { return new(commonv1.KeyValueList) },

		// resource/v1
		"Resource": func() unmarshaler { return new(resourcev1.Resource) },

		// trace/v1
		"TracesData":    func() unmarshaler { return new(tracev1.TracesData) },
		"ResourceSpans": func() unmarshaler { return new(tracev1.ResourceSpans) },
		"ScopeSpans":    func() unmarshaler { return new(tracev1.ScopeSpans) },
		"Span":          func() unmarshaler { return new(tracev1.Span) },
		"Span_Event":    func() unmarshaler { return new(tracev1.Span_Event) },
		"Span_Link":     func() unmarshaler { return new(tracev1.Span_Link) },
		"Status":        func() unmarshaler { return new(tracev1.Status) },

		// logs/v1
		"LogsData":     func() unmarshaler { return new(logsv1.LogsData) },
		"ResourceLogs": func() unmarshaler { return new(logsv1.ResourceLogs) },
		"ScopeLogs":    func() unmarshaler { return new(logsv1.ScopeLogs) },
		"LogRecord":    func() unmarshaler { return new(logsv1.LogRecord) },

		// metrics/v1
		"MetricsData":                           func() unmarshaler { return new(metricsv1.MetricsData) },
		"ResourceMetrics":                       func() unmarshaler { return new(metricsv1.ResourceMetrics) },
		"ScopeMetrics":                          func() unmarshaler { return new(metricsv1.ScopeMetrics) },
		"Metric":                                func() unmarshaler { return new(metricsv1.Metric) },
		"Gauge":                                 func() unmarshaler { return new(metricsv1.Gauge) },
		"Sum":                                   func() unmarshaler { return new(metricsv1.Sum) },
		"Histogram":                             func() unmarshaler { return new(metricsv1.Histogram) },
		"HistogramDataPoint":                    func() unmarshaler { return new(metricsv1.HistogramDataPoint) },
		"ExponentialHistogram":                  func() unmarshaler { return new(metricsv1.ExponentialHistogram) },
		"ExponentialHistogramDataPoint":         func() unmarshaler { return new(metricsv1.ExponentialHistogramDataPoint) },
		"ExponentialHistogramDataPoint_Buckets": func() unmarshaler { return new(metricsv1.ExponentialHistogramDataPoint_Buckets) },
		"Summary":                               func() unmarshaler { return new(metricsv1.Summary) },
		"SummaryDataPoint":                      func() unmarshaler { return new(metricsv1.SummaryDataPoint) },
		"SummaryDataPoint_ValueAtQuantile":      func() unmarshaler { return new(metricsv1.SummaryDataPoint_ValueAtQuantile) },
		"NumberDataPoint":                       func() unmarshaler { return new(metricsv1.NumberDataPoint) },
		"Exemplar":                              func() unmarshaler { return new(metricsv1.Exemplar) },

		// profiles/v1development
		"ProfilesData":       func() unmarshaler { return new(profilesv1.ProfilesData) },
		"ResourceProfiles":   func() unmarshaler { return new(profilesv1.ResourceProfiles) },
		"ScopeProfiles":      func() unmarshaler { return new(profilesv1.ScopeProfiles) },
		"Profile":            func() unmarshaler { return new(profilesv1.Profile) },
		"ProfilesDictionary": func() unmarshaler { return new(profilesv1.ProfilesDictionary) },
		"ValueType":          func() unmarshaler { return new(profilesv1.ValueType) },
		"Sample":             func() unmarshaler { return new(profilesv1.Sample) },
		"Mapping":            func() unmarshaler { return new(profilesv1.Mapping) },
		"Location":           func() unmarshaler { return new(profilesv1.Location) },
		"Line":               func() unmarshaler { return new(profilesv1.Line) },
		"Function":           func() unmarshaler { return new(profilesv1.Function) },
		"Link":               func() unmarshaler { return new(profilesv1.Link) },
		"Stack":              func() unmarshaler { return new(profilesv1.Stack) },
	}
}

// FuzzUnmarshal feeds random bytes into every generated Unmarshal
// method. The test passes as long as no method panics — errors are expected.
func FuzzUnmarshal(f *testing.F) {
	// Seed corpus with interesting byte patterns.
	f.Add([]byte{})                                                     // empty
	f.Add([]byte{0x08, 0x01})                                           // valid varint field
	f.Add([]byte{0x0a, 0x00})                                           // valid empty bytes field
	f.Add([]byte{0x80})                                                 // truncated tag
	f.Add([]byte{0x08, 0x80})                                           // truncated varint
	f.Add([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff})                   // overflow varint
	f.Add([]byte{0x0a, 0xff, 0xff, 0xff, 0xff, 0x0f})                   // huge length prefix
	f.Add([]byte{0x0d, 0x01, 0x02, 0x03, 0x04})                         // fixed32 field
	f.Add([]byte{0x09, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}) // fixed64 field

	ctors := allMessageConstructors()

	f.Fuzz(func(t *testing.T, data []byte) {
		for name, newMsg := range ctors {
			msg := newMsg()
			// We only care that it doesn't panic; errors are fine.
			_ = msg.Unmarshal(data)
			_ = name
		}
	})
}
