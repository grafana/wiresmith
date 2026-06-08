// Package peer — gRPC compatibility verification for the split-file
// service-only case (regression coverage for wiresmith-e9h).
//
// The companion grpc_protoc_compat_test.go exercises the same scenarios
// against `basic/service/v1`, where the proto file co-locates the
// request/response messages with the service definition. That layout
// hides the bug fixed in wiresmith-e9h, because `shouldGenerateFile`
// would emit the file regardless of whether services existed — it had
// messages.
//
// This test mirrors the same harness against `serviceonly/v1`, where
// `service.proto` declares only an `EchoSplit` service that references
// `Request` / `Response` from a sibling `messages.proto`. Pre-fix,
// `service.proto` was dropped from both the collision-reservation loop
// and the emit dispatch, so `_grpc.pb.go` was never produced for it.
// Post-fix, the stubs land at `gen/serviceonly/v1/service_grpc.pb.go`
// and the end-to-end gRPC dial below succeeds.
package peer

import (
	"context"
	"errors"
	"io"
	"net"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/test/bufconn"

	serviceonlyv1 "github.com/grafana/wiresmith/gen/serviceonly/v1"
)

type echoSplitServer struct {
	serviceonlyv1.UnimplementedEchoSplitServer
}

func (echoSplitServer) Unary(_ context.Context, in *serviceonlyv1.Request) (*serviceonlyv1.Response, error) {
	return &serviceonlyv1.Response{Id: in.Id, PayloadBytes: int64(len(in.Payload))}, nil
}

func (echoSplitServer) ClientStream(stream grpc.ClientStreamingServer[serviceonlyv1.Request, serviceonlyv1.Response]) error {
	resp := &serviceonlyv1.Response{}
	for {
		m, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if resp.Id == "" {
			resp.Id = m.Id
		}
		resp.PayloadBytes += int64(len(m.Payload))
	}
	return stream.SendAndClose(resp)
}

func (echoSplitServer) ServerStream(in *serviceonlyv1.Request, stream grpc.ServerStreamingServer[serviceonlyv1.Response]) error {
	for i := range 4 {
		out := &serviceonlyv1.Response{Id: in.Id, PayloadBytes: int64(len(in.Payload)) + int64(i)}
		if err := stream.Send(out); err != nil {
			return err
		}
	}
	return nil
}

func (echoSplitServer) BidiStream(stream grpc.BidiStreamingServer[serviceonlyv1.Request, serviceonlyv1.Response]) error {
	for {
		m, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		out := &serviceonlyv1.Response{Id: m.Id, PayloadBytes: int64(len(m.Payload))}
		if err := stream.Send(out); err != nil {
			return err
		}
	}
}

func sampleRequest() *serviceonlyv1.Request {
	return &serviceonlyv1.Request{Id: "id-7", Payload: []byte("split-fixture-payload")}
}

func newEchoSplitHarness(t *testing.T) (serviceonlyv1.EchoSplitClient, *grpc.Server) {
	t.Helper()
	lis := bufconn.Listen(bufSize)
	srv := grpc.NewServer()
	serviceonlyv1.RegisterEchoSplitServer(srv, echoSplitServer{})
	reflection.Register(srv)
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- srv.Serve(lis)
	}()
	t.Cleanup(func() {
		srv.Stop()
		if err := <-serveErr; err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Errorf("grpc Serve: %v", err)
		}
	})

	dial := func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(dial),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Logf("grpc client Close: %v", err)
		}
	})
	return serviceonlyv1.NewEchoSplitClient(conn), srv
}

func TestGRPCProtocCompatServiceOnly_Unary(t *testing.T) {
	cli, _ := newEchoSplitHarness(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	in := sampleRequest()
	out, err := cli.Unary(ctx, in)
	require.NoError(t, err)
	require.Equal(t, in.Id, out.Id)
	require.Equal(t, int64(len(in.Payload)), out.PayloadBytes)
}

func TestGRPCProtocCompatServiceOnly_ClientStream(t *testing.T) {
	cli, _ := newEchoSplitHarness(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := cli.ClientStream(ctx)
	require.NoError(t, err)

	var totalBytes int64
	const n = 5
	for range n {
		r := sampleRequest()
		require.NoError(t, stream.Send(r))
		totalBytes += int64(len(r.Payload))
	}
	resp, err := stream.CloseAndRecv()
	require.NoError(t, err)
	require.Equal(t, totalBytes, resp.PayloadBytes)
	require.Equal(t, "id-7", resp.Id)
}

func TestGRPCProtocCompatServiceOnly_ServerStream(t *testing.T) {
	cli, _ := newEchoSplitHarness(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	in := sampleRequest()
	stream, err := cli.ServerStream(ctx, in)
	require.NoError(t, err)

	var got []int64
	for {
		m, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		require.Equal(t, in.Id, m.Id)
		got = append(got, m.PayloadBytes)
	}
	base := int64(len(in.Payload))
	require.Equal(t, []int64{base, base + 1, base + 2, base + 3}, got)
}

func TestGRPCProtocCompatServiceOnly_BidiStream(t *testing.T) {
	cli, _ := newEchoSplitHarness(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := cli.BidiStream(ctx)
	require.NoError(t, err)

	const n = 4
	sent := make([]*serviceonlyv1.Request, n)
	for i := range n {
		r := sampleRequest()
		r.Id = "msg-" + string(rune('a'+i))
		sent[i] = r
		require.NoError(t, stream.Send(r))
	}
	require.NoError(t, stream.CloseSend())

	for i := range n {
		got, err := stream.Recv()
		require.NoError(t, err)
		require.Equal(t, sent[i].Id, got.Id)
		require.Equal(t, int64(len(sent[i].Payload)), got.PayloadBytes)
	}
	_, err = stream.Recv()
	require.ErrorIs(t, err, io.EOF)
}

// TestGRPCProtocCompatServiceOnly_ServiceInfo pins the live ServiceRegistrar
// view of the EchoSplit service. Mirrors the companion service.proto fixture
// test in grpc_protoc_compat_test.go; the duplication is intentional — the
// whole point of this test file is to prove that the stubs generated for
// the split service-only proto are reachable through the same registration
// path as those generated for the co-located proto.
func TestGRPCProtocCompatServiceOnly_ServiceInfo(t *testing.T) {
	_, srv := newEchoSplitHarness(t)

	info := srv.GetServiceInfo()
	names := make([]string, 0, len(info))
	for name := range info {
		names = append(names, name)
	}
	sort.Strings(names)

	require.Contains(t, names, "basic.serviceonly.v1.EchoSplit")
	require.Contains(t, names, "grpc.reflection.v1.ServerReflection")

	echo := info["basic.serviceonly.v1.EchoSplit"]
	methods := make([]string, 0, len(echo.Methods))
	for _, m := range echo.Methods {
		methods = append(methods, m.Name)
	}
	sort.Strings(methods)
	require.Equal(t, []string{"BidiStream", "ClientStream", "ServerStream", "Unary"}, methods)
}
