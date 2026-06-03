// Package peer — gRPC compatibility verification for wiresmith messages used
// with stubs emitted by protoc-gen-go-grpc.
//
// The hypothesis (wiresmith-dei) is that protoc-gen-go-grpc's emitted client +
// server stubs interoperate end-to-end with wiresmith-generated message types,
// because:
//
//  1. wiresmith messages register a protoimpl.MessageInfo fast-path Methods
//     table (see gen/protohelpers/message.go::wiresmithMethods).
//  2. google.golang.org/grpc/encoding/proto's codec calls proto.Marshal /
//     proto.Unmarshal, which dispatches through that Methods table — no
//     reflection slow-path on the gRPC hot path.
//  3. protoc-gen-go-grpc emits only client/server stubs and a grpc.ServiceDesc;
//     it does not touch protoregistry.GlobalFiles, so wiresmith and
//     protoc-gen-go-grpc can both target the same Go package without
//     registration conflicts.
//
// These tests exercise all four streaming modes and a reflection sanity check;
// any reflection-only field-level access through wiresmith's MessageReflect
// would panic loudly (see gen/protohelpers/message.go::panicReflect).
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

	servicepb "wiresmith/gen/basic/service/v1"
)

const bufSize = 1024 * 1024

type echoServer struct {
	servicepb.UnimplementedEchoServer
}

func (echoServer) Unary(_ context.Context, in *servicepb.Payload) (*servicepb.Payload, error) {
	out := *in
	return &out, nil
}

func (echoServer) ClientStream(stream grpc.ClientStreamingServer[servicepb.Payload, servicepb.Payload]) error {
	acc := &servicepb.Payload{}
	for {
		m, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		acc.Sequence += m.Sequence
		if acc.Id == "" {
			acc.Id = m.Id
		}
	}
	return stream.SendAndClose(acc)
}

func (echoServer) ServerStream(in *servicepb.Payload, stream grpc.ServerStreamingServer[servicepb.Payload]) error {
	for i := int64(0); i < 4; i++ {
		clone := *in
		clone.Sequence = in.Sequence + i
		if err := stream.Send(&clone); err != nil {
			return err
		}
	}
	return nil
}

func (echoServer) BidiStream(stream grpc.BidiStreamingServer[servicepb.Payload, servicepb.Payload]) error {
	for {
		m, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := stream.Send(m); err != nil {
			return err
		}
	}
}

func samplePayload() *servicepb.Payload {
	return &servicepb.Payload{
		Id:       "id-42",
		Sequence: 12345,
		Chunks:   [][]byte{[]byte("alpha"), {0x00, 0xff, 0x7f}},
		Labels:   map[string]string{"env": "prod", "team": "fleet"},
		Kind:     &servicepb.Payload_Text{Text: "hello"},
		Nested:   servicepb.Nested{Name: "n", Value: 3.14},
		Status:   servicepb.Payload_STATUS_OK,
	}
}

func newEchoHarness(t *testing.T) (servicepb.EchoClient, *grpc.Server) {
	t.Helper()
	lis := bufconn.Listen(bufSize)
	srv := grpc.NewServer()
	servicepb.RegisterEchoServer(srv, echoServer{})
	reflection.Register(srv)
	// Serve returns once Stop / GracefulStop is called; that path commonly
	// surfaces as grpc.ErrServerStopped, not a nil error. Capture the
	// result on a channel and consume it in t.Cleanup so the goroutine
	// cannot outlive the test — calling t.Errorf from a goroutine that
	// races past the test's completion would otherwise trigger "Log in
	// goroutine after Test... has completed".
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- srv.Serve(lis)
	}()
	// Register server-side cleanup BEFORE the first require.NoError so
	// the listener and Serve goroutine are still torn down if the
	// client-construction step below fails and aborts the test.
	t.Cleanup(func() {
		srv.Stop()
		if err := <-serveErr; err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Errorf("grpc Serve: %v", err)
		}
	})

	dial := func(context.Context, string) (net.Conn, error) { return lis.Dial() }
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(dial),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Logf("grpc client Close: %v", err)
		}
	})
	return servicepb.NewEchoClient(conn), srv
}

func TestGRPCProtocCompat_Unary(t *testing.T) {
	cli, _ := newEchoHarness(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	in := samplePayload()
	out, err := cli.Unary(ctx, in)
	require.NoError(t, err)
	require.True(t, in.Equal(out), "unary roundtrip mismatch: got %+v, want %+v", out, in)
}

func TestGRPCProtocCompat_ClientStream(t *testing.T) {
	cli, _ := newEchoHarness(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := cli.ClientStream(ctx)
	require.NoError(t, err)

	var sumExpected int64
	const n = 5
	for i := int64(1); i <= n; i++ {
		p := samplePayload()
		p.Sequence = i
		require.NoError(t, stream.Send(p))
		sumExpected += i
	}
	resp, err := stream.CloseAndRecv()
	require.NoError(t, err)
	require.Equal(t, sumExpected, resp.Sequence, "client-stream sum mismatch")
	require.Equal(t, "id-42", resp.Id, "id field should be carried from first message")
}

func TestGRPCProtocCompat_ServerStream(t *testing.T) {
	cli, _ := newEchoHarness(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	in := samplePayload()
	in.Sequence = 100
	stream, err := cli.ServerStream(ctx, in)
	require.NoError(t, err)

	var got []int64
	for {
		m, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		// Build an expected clone whose Sequence matches the server's bump;
		// every other field must survive the roundtrip. Use wiresmith's Equal
		// (semantic comparison) rather than require.Equal, because freshly
		// unmarshaled value-type fields carry presence-bitmap bits that an
		// in-memory-constructed expected struct does not.
		want := *in
		want.Sequence = m.Sequence
		require.True(t, want.Equal(m), "server-stream payload mismatch: got %+v, want %+v", m, &want)
		got = append(got, m.Sequence)
	}
	require.Equal(t, []int64{100, 101, 102, 103}, got)
}

func TestGRPCProtocCompat_BidiStream(t *testing.T) {
	cli, _ := newEchoHarness(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := cli.BidiStream(ctx)
	require.NoError(t, err)

	const n = 4
	sent := make([]*servicepb.Payload, n)
	for i := range n {
		p := samplePayload()
		p.Sequence = int64(i)
		// Alternate oneof variant to exercise both branches.
		if i%2 == 1 {
			p.Kind = &servicepb.Payload_Number{Number: int64(i) * 7}
		}
		sent[i] = p
		require.NoError(t, stream.Send(p))
	}
	require.NoError(t, stream.CloseSend())

	for i := range n {
		got, err := stream.Recv()
		require.NoError(t, err)
		require.True(t, sent[i].Equal(got),
			"bidi roundtrip mismatch at i=%d: got %+v, want %+v", i, got, sent[i])
	}
	// Server closes its half after CloseSend; expect EOF.
	_, err = stream.Recv()
	require.ErrorIs(t, err, io.EOF)
}

// TestGRPCProtocCompat_ServiceInfo asserts the gRPC server's own service
// registry — populated by the protoc-gen-go-grpc-emitted RegisterEchoServer —
// reports the wiresmith-backed Echo service. This is the path that powers the
// reflection v1 ListServices RPC (reflection/internal/serverreflection.go
// uses Server.GetServiceInfo, not protoregistry.GlobalFiles).
func TestGRPCProtocCompat_ServiceInfo(t *testing.T) {
	_, srv := newEchoHarness(t)

	info := srv.GetServiceInfo()
	names := make([]string, 0, len(info))
	for name := range info {
		names = append(names, name)
	}
	sort.Strings(names)

	// The Echo service must be present (the whole point of this PR), and
	// the gRPC reflection service must also be present (newEchoHarness
	// installs it via reflection.Register). Asserting both keeps the
	// listing honest if a future refactor drops one of them.
	require.Contains(t, names, "basic.service.v1.Echo")
	require.Contains(t, names, "grpc.reflection.v1.ServerReflection")

	echo := info["basic.service.v1.Echo"]
	methods := make([]string, 0, len(echo.Methods))
	for _, m := range echo.Methods {
		methods = append(methods, m.Name)
	}
	sort.Strings(methods)
	require.Equal(t, []string{"BidiStream", "ClientStream", "ServerStream", "Unary"}, methods)
}
