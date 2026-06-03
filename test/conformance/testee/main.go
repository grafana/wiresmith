// Conformance testee for wiresmith.
//
// Speaks the protobuf conformance test protocol: reads ConformanceRequest
// messages from stdin (4-byte LE length prefix), processes them, and writes
// ConformanceResponse messages to stdout.
//
// Uses wiresmith-generated code for TestAllTypesProto3 and standard
// google.golang.org/protobuf for the conformance protocol envelope.
package main

import (
	"encoding/binary"
	"io"
	"os"

	"google.golang.org/protobuf/proto"

	testmsg "github.com/grafana/wiresmith/gen/protobuf_test_messages/proto3"
	pb "github.com/grafana/wiresmith/test/conformance/internal/conformancepb"
)

func main() {
	for {
		req, err := readRequest(os.Stdin)
		if err == io.EOF {
			os.Exit(0)
		}
		if err != nil {
			os.Exit(1)
		}

		resp := handle(req)

		if err := writeResponse(os.Stdout, resp); err != nil {
			os.Exit(1)
		}
	}
}

func handle(req *pb.ConformanceRequest) *pb.ConformanceResponse {
	// Only support binary protobuf input.
	payload := req.GetProtobufPayload()
	if payload == nil {
		return skipped("only protobuf input supported")
	}

	// Only support binary protobuf output.
	if req.RequestedOutputFormat != pb.WireFormat_PROTOBUF {
		return skipped("only protobuf output supported")
	}

	// Only support TestAllTypesProto3.
	if req.MessageType != "protobuf_test_messages.proto3.TestAllTypesProto3" {
		return skipped("unsupported message type: " + req.MessageType)
	}

	var msg testmsg.TestAllTypesProto3
	if err := msg.Unmarshal(payload); err != nil {
		return &pb.ConformanceResponse{
			Result: &pb.ConformanceResponse_ParseError{
				ParseError: err.Error(),
			},
		}
	}

	out, err := msg.Marshal()
	if err != nil {
		return &pb.ConformanceResponse{
			Result: &pb.ConformanceResponse_SerializeError{
				SerializeError: err.Error(),
			},
		}
	}

	return &pb.ConformanceResponse{
		Result: &pb.ConformanceResponse_ProtobufPayload{
			ProtobufPayload: out,
		},
	}
}

func skipped(reason string) *pb.ConformanceResponse {
	return &pb.ConformanceResponse{
		Result: &pb.ConformanceResponse_Skipped{
			Skipped: reason,
		},
	}
}

func readRequest(r io.Reader) (*pb.ConformanceRequest, error) {
	var length uint32
	if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
		return nil, err
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	var req pb.ConformanceRequest
	if err := proto.Unmarshal(buf, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

func writeResponse(w io.Writer, resp *pb.ConformanceResponse) error {
	buf, err := proto.Marshal(resp)
	if err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, uint32(len(buf))); err != nil {
		return err
	}
	_, err = w.Write(buf)
	return err
}
