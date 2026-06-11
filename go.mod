module github.com/grafana/wiresmith

go 1.26.4

require (
	github.com/bufbuild/protocompile v0.14.1
	github.com/gogo/protobuf v1.3.2
	// Post-v0.6.0 pseudo-version, pulled in transitively via
	// google.golang.org/grpc's go.mod. wiresmith only uses vtprotobuf's
	// `protohelpers` package, which is unchanged between v0.6.0 and this
	// commit, so the bump is a benign dependency update. No `replace` is
	// used, which keeps the module `go install`-able.
	github.com/planetscale/vtprotobuf v0.6.1-0.20250313105119-ba97887b0a25
	github.com/stretchr/testify v1.11.1
	go.opentelemetry.io/collector/pdata v1.59.0
	go.opentelemetry.io/proto/otlp v1.10.0
	google.golang.org/grpc v1.81.1
	google.golang.org/protobuf v1.36.11
)

require github.com/google/go-cmp v0.7.0

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/hashicorp/go-version v1.9.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	go.opentelemetry.io/collector/featuregate v1.59.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/text v0.37.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260226221140-a57be14db171 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
