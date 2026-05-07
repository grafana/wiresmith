package testutil

import (
	commonv1 "wiresmith/gen/otlp/common/v1"
	logsv1 "wiresmith/gen/otlp/logs/v1"
	metricsv1 "wiresmith/gen/otlp/metrics/v1"
	profilesv1 "wiresmith/gen/otlp/profiles/v1development"
	resourcev1 "wiresmith/gen/otlp/resource/v1"
	tracev1 "wiresmith/gen/otlp/trace/v1"
)

// Message is implemented by all generated message types.
type Message interface {
	Unmarshal([]byte) error
	Marshal() ([]byte, error)
	Size() int
}

// AllMessageConstructors returns a constructor for every OTLP generated message type.
func AllMessageConstructors() map[string]func() Message {
	return map[string]func() Message{
		// common/v1
		"AnyValue":             func() Message { return new(commonv1.AnyValue) },
		"ArrayValue":           func() Message { return new(commonv1.ArrayValue) },
		"InstrumentationScope": func() Message { return new(commonv1.InstrumentationScope) },
		"KeyValue":             func() Message { return new(commonv1.KeyValue) },
		"KeyValueList":         func() Message { return new(commonv1.KeyValueList) },

		// resource/v1
		"Resource": func() Message { return new(resourcev1.Resource) },

		// trace/v1
		"TracesData":    func() Message { return new(tracev1.TracesData) },
		"ResourceSpans": func() Message { return new(tracev1.ResourceSpans) },
		"ScopeSpans":    func() Message { return new(tracev1.ScopeSpans) },
		"Span":          func() Message { return new(tracev1.Span) },
		"Span_Event":    func() Message { return new(tracev1.Span_Event) },
		"Span_Link":     func() Message { return new(tracev1.Span_Link) },
		"Status":        func() Message { return new(tracev1.Status) },

		// logs/v1
		"LogsData":     func() Message { return new(logsv1.LogsData) },
		"ResourceLogs": func() Message { return new(logsv1.ResourceLogs) },
		"ScopeLogs":    func() Message { return new(logsv1.ScopeLogs) },
		"LogRecord":    func() Message { return new(logsv1.LogRecord) },

		// metrics/v1
		"MetricsData":                           func() Message { return new(metricsv1.MetricsData) },
		"ResourceMetrics":                       func() Message { return new(metricsv1.ResourceMetrics) },
		"ScopeMetrics":                          func() Message { return new(metricsv1.ScopeMetrics) },
		"Metric":                                func() Message { return new(metricsv1.Metric) },
		"Gauge":                                 func() Message { return new(metricsv1.Gauge) },
		"Sum":                                   func() Message { return new(metricsv1.Sum) },
		"Histogram":                             func() Message { return new(metricsv1.Histogram) },
		"HistogramDataPoint":                    func() Message { return new(metricsv1.HistogramDataPoint) },
		"ExponentialHistogram":                  func() Message { return new(metricsv1.ExponentialHistogram) },
		"ExponentialHistogramDataPoint":         func() Message { return new(metricsv1.ExponentialHistogramDataPoint) },
		"ExponentialHistogramDataPoint_Buckets": func() Message { return new(metricsv1.ExponentialHistogramDataPoint_Buckets) },
		"Summary":                               func() Message { return new(metricsv1.Summary) },
		"SummaryDataPoint":                      func() Message { return new(metricsv1.SummaryDataPoint) },
		"SummaryDataPoint_ValueAtQuantile":      func() Message { return new(metricsv1.SummaryDataPoint_ValueAtQuantile) },
		"NumberDataPoint":                       func() Message { return new(metricsv1.NumberDataPoint) },
		"Exemplar":                              func() Message { return new(metricsv1.Exemplar) },

		// profiles/v1development
		"ProfilesData":       func() Message { return new(profilesv1.ProfilesData) },
		"ResourceProfiles":   func() Message { return new(profilesv1.ResourceProfiles) },
		"ScopeProfiles":      func() Message { return new(profilesv1.ScopeProfiles) },
		"Profile":            func() Message { return new(profilesv1.Profile) },
		"ProfilesDictionary": func() Message { return new(profilesv1.ProfilesDictionary) },
		"ValueType":          func() Message { return new(profilesv1.ValueType) },
		"Sample":             func() Message { return new(profilesv1.Sample) },
		"Mapping":            func() Message { return new(profilesv1.Mapping) },
		"Location":           func() Message { return new(profilesv1.Location) },
		"Line":               func() Message { return new(profilesv1.Line) },
		"Function":           func() Message { return new(profilesv1.Function) },
		"Link":               func() Message { return new(profilesv1.Link) },
		"Stack":              func() Message { return new(profilesv1.Stack) },
	}
}
