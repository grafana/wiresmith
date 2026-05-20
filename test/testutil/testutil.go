package testutil

import (
	"maps"
	enumv1 "wiresmith/gen/basic/enum/v1"
	mapsv1 "wiresmith/gen/basic/maps/v1"
	nestingv1 "wiresmith/gen/basic/nesting/v1"
	numericv1 "wiresmith/gen/basic/numeric/v1"
	oneofv1 "wiresmith/gen/basic/oneof/v1"
	recursivev1 "wiresmith/gen/basic/recursive/v1"
	commonv1 "wiresmith/gen/otlp/common/v1"
	logsv1 "wiresmith/gen/otlp/logs/v1"
	metricsv1 "wiresmith/gen/otlp/metrics/v1"
	profilesv1 "wiresmith/gen/otlp/profiles/v1development"
	resourcev1 "wiresmith/gen/otlp/resource/v1"
	tracev1 "wiresmith/gen/otlp/trace/v1"
	kitchensinkv1 "wiresmith/gen/test/kitchensink/v1"
)

// Message is implemented by all generated message types.
type Message interface {
	Unmarshal([]byte) error
	Marshal() ([]byte, error)
	Size() int
}

// AllMessageConstructors returns a constructor for every generated message
// type whose marshal output is byte-deterministic across runs (i.e. has no
// transitively-reachable map field). Suitable for tests that compare bytes
// after a round-trip. For map-bearing types, see MapBearingMessageConstructors.
func AllMessageConstructors() map[string]func() Message {
	out := otlpMessageConstructors()
	maps.Copy(out, basicMapFreeMessageConstructors())
	return out
}

// MapBearingMessageConstructors returns constructors for generated types that
// (transitively) contain map fields. Map iteration order is randomized in Go,
// so byte-equal round-trip assertions are unsafe for these types — use them
// in panic-only or size-only checks.
func MapBearingMessageConstructors() map[string]func() Message {
	return map[string]func() Message{
		"basic.MapBench":            func() Message { return new(mapsv1.MapBench) },
		"basic.EnumContainer":       func() Message { return new(enumv1.EnumContainer) },
		"basic.OneofPlusEverything": func() Message { return new(oneofv1.OneofPlusEverything) },
		"kitchensink.AllMaps":       func() Message { return new(kitchensinkv1.AllMaps) },
	}
}

// AllPanicSafeConstructors returns the union of map-free and map-bearing
// constructors. Use it for tests that only need to verify "Unmarshal does not
// panic" or "Size() == len(Marshal())".
func AllPanicSafeConstructors() map[string]func() Message {
	out := AllMessageConstructors()
	maps.Copy(out, MapBearingMessageConstructors())
	return out
}

// basicMapFreeMessageConstructors returns generated non-OTLP types that do not
// contain any map fields (transitively), so their marshal output is
// byte-deterministic.
func basicMapFreeMessageConstructors() map[string]func() Message {
	return map[string]func() Message{
		// basic/recursive: self-referential pointer + slice
		"basic.LinkedList": func() Message { return new(recursivev1.LinkedList) },
		"basic.TreeNode":   func() Message { return new(recursivev1.TreeNode) },
		"basic.NodeA":      func() Message { return new(recursivev1.NodeA) },
		"basic.NodeB":      func() Message { return new(recursivev1.NodeB) },

		// basic/numeric: unpacked repeated, mixed modifiers, wide bitmap
		"basic.UnpackedScalars": func() Message { return new(numericv1.UnpackedScalars) },
		"basic.MixedModifiers":  func() Message { return new(numericv1.MixedModifiers) },
		"basic.WideFields":      func() Message { return new(numericv1.WideFields) },

		// basic/nesting: 4-level inline messages
		"basic.Level0":   func() Message { return new(nestingv1.Level0) },
		"basic.CrossRef": func() Message { return new(nestingv1.CrossRef) },

		// basic/enum: nested enum (EnumContainer omitted — has signed_map)
		"basic.WithNestedEnum": func() Message { return new(enumv1.WithNestedEnum) },

		// basic/maps: map value-type message (no maps of its own)
		"basic.maps.Inner": func() Message { return new(mapsv1.Inner) },

		// basic/oneof: payload, multi-oneof, oneof with message/enum variants
		"basic.Payload":        func() Message { return new(oneofv1.Payload) },
		"basic.MultiOneof":     func() Message { return new(oneofv1.MultiOneof) },
		"basic.OneofWithTypes": func() Message { return new(oneofv1.OneofWithTypes) },

		// test/kitchensink: every scalar shape and oneof type
		"kitchensink.AllScalars":         func() Message { return new(kitchensinkv1.AllScalars) },
		"kitchensink.AllOptionalScalars": func() Message { return new(kitchensinkv1.AllOptionalScalars) },
		"kitchensink.AllRepeatedScalars": func() Message { return new(kitchensinkv1.AllRepeatedScalars) },
		"kitchensink.OneofVariants":      func() Message { return new(kitchensinkv1.OneofVariants) },
		"kitchensink.Outer":              func() Message { return new(kitchensinkv1.Outer) },
		"kitchensink.Middle":             func() Message { return new(kitchensinkv1.Middle) },
		"kitchensink.Inner":              func() Message { return new(kitchensinkv1.Inner) },
		"kitchensink.HighFieldNumbers":   func() Message { return new(kitchensinkv1.HighFieldNumbers) },
		"kitchensink.WithEnum":           func() Message { return new(kitchensinkv1.WithEnum) },
		"kitchensink.Empty":              func() Message { return new(kitchensinkv1.Empty) },
		"kitchensink.OnlyRepeated":       func() Message { return new(kitchensinkv1.OnlyRepeated) },
		"kitchensink.Container":          func() Message { return new(kitchensinkv1.Container) },
	}
}

// otlpMessageConstructors returns the OTLP generated message types. Kept
// separate so AllMessageConstructors can compose with basic types and a future
// caller can request OTLP-only if needed.
func otlpMessageConstructors() map[string]func() Message {
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
