package generator

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// emitAllGRPCServices generates gRPC client/server stubs for all services in the file.
func (fg *FileGenerator) emitAllGRPCServices(fd protoreflect.FileDescriptor) {
	if fd.Services().Len() == 0 {
		return
	}

	fg.imports.addStdImport("context")
	fg.imports.addImport("google.golang.org/grpc", "grpc")
	fg.imports.addImport("google.golang.org/grpc/codes", "codes")
	fg.imports.addImport("google.golang.org/grpc/status", "status")

	for i := 0; i < fd.Services().Len(); i++ {
		fg.emitGRPCService(fd.Services().Get(i))
	}
}

func (fg *FileGenerator) emitGRPCService(sd protoreflect.ServiceDescriptor) {
	svcName := string(sd.Name())

	fg.emitGRPCClient(sd, svcName)
	fg.emitGRPCServer(sd, svcName)
	fg.emitGRPCHandlers(sd, svcName)
	fg.emitGRPCServiceDesc(sd, svcName)
}

func (fg *FileGenerator) emitGRPCClient(sd protoreflect.ServiceDescriptor, svcName string) {
	lowerName := toLowerFirst(svcName)

	// Client interface
	fmt.Fprintf(fg.body, "// %sClient is the client API for %s service.\n", svcName, svcName)
	fmt.Fprintf(fg.body, "type %sClient interface {\n", svcName)
	for i := 0; i < sd.Methods().Len(); i++ {
		md := sd.Methods().Get(i)
		fg.emitClientMethodSignature(md, "\t")
		fg.body.WriteString("\n")
	}
	fmt.Fprintf(fg.body, "}\n\n")

	// Client struct
	fmt.Fprintf(fg.body, "type %sClient struct {\n", lowerName)
	fmt.Fprintf(fg.body, "\tcc *grpc.ClientConn\n")
	fmt.Fprintf(fg.body, "}\n\n")

	// Constructor
	fmt.Fprintf(fg.body, "func New%sClient(cc *grpc.ClientConn) %sClient {\n", svcName, svcName)
	fmt.Fprintf(fg.body, "\treturn &%sClient{cc}\n", lowerName)
	fmt.Fprintf(fg.body, "}\n\n")

	// Client methods
	for i := 0; i < sd.Methods().Len(); i++ {
		md := sd.Methods().Get(i)
		methodName := string(md.Name())
		fullMethod := fmt.Sprintf("/%s.%s/%s", sd.ParentFile().Package(), svcName, methodName)
		inType := fg.grpcMessageType(md.Input())
		outType := fg.grpcMessageType(md.Output())

		if md.IsStreamingClient() || md.IsStreamingServer() {
			fg.emitStreamingClientMethod(sd, md, lowerName, svcName, fullMethod)
		} else {
			fmt.Fprintf(fg.body, "func (c *%sClient) %s(ctx context.Context, in *%s, opts ...grpc.CallOption) (*%s, error) {\n",
				lowerName, methodName, inType, outType)
			fmt.Fprintf(fg.body, "\tout := new(%s)\n", outType)
			fmt.Fprintf(fg.body, "\terr := c.cc.Invoke(ctx, %q, in, out, opts...)\n", fullMethod)
			fmt.Fprintf(fg.body, "\tif err != nil {\n\t\treturn nil, err\n\t}\n")
			fmt.Fprintf(fg.body, "\treturn out, nil\n")
			fmt.Fprintf(fg.body, "}\n\n")
		}
	}
}

func (fg *FileGenerator) emitGRPCServer(sd protoreflect.ServiceDescriptor, svcName string) {
	// Server interface
	fmt.Fprintf(fg.body, "// %sServer is the server API for %s service.\n", svcName, svcName)
	fmt.Fprintf(fg.body, "type %sServer interface {\n", svcName)
	for i := 0; i < sd.Methods().Len(); i++ {
		md := sd.Methods().Get(i)
		fg.emitServerMethodSignature(md, "\t")
		fg.body.WriteString("\n")
	}
	fmt.Fprintf(fg.body, "}\n\n")

	// Unimplemented server
	fmt.Fprintf(fg.body, "// Unimplemented%sServer can be embedded to have forward compatible implementations.\n", svcName)
	fmt.Fprintf(fg.body, "type Unimplemented%sServer struct{}\n\n", svcName)

	for i := 0; i < sd.Methods().Len(); i++ {
		md := sd.Methods().Get(i)
		methodName := string(md.Name())
		outType := fg.grpcMessageType(md.Output())

		if md.IsStreamingServer() {
			streamType := svcName + "_" + methodName + "Server"
			fmt.Fprintf(fg.body, "func (*Unimplemented%sServer) %s(*%s, %s) error {\n",
				svcName, methodName, fg.grpcMessageType(md.Input()), streamType)
		} else {
			fmt.Fprintf(fg.body, "func (*Unimplemented%sServer) %s(ctx context.Context, req *%s) (*%s, error) {\n",
				svcName, methodName, fg.grpcMessageType(md.Input()), outType)
		}
		fmt.Fprintf(fg.body, "\treturn nil, status.Errorf(codes.Unimplemented, \"method %s not implemented\")\n", methodName)
		fmt.Fprintf(fg.body, "}\n\n")
	}

	// Register function
	fmt.Fprintf(fg.body, "func Register%sServer(s *grpc.Server, srv %sServer) {\n", svcName, svcName)
	fmt.Fprintf(fg.body, "\ts.RegisterService(&_%s_serviceDesc, srv)\n", svcName)
	fmt.Fprintf(fg.body, "}\n\n")
}

func (fg *FileGenerator) emitGRPCHandlers(sd protoreflect.ServiceDescriptor, svcName string) {
	for i := 0; i < sd.Methods().Len(); i++ {
		md := sd.Methods().Get(i)
		methodName := string(md.Name())
		inType := fg.grpcMessageType(md.Input())

		if md.IsStreamingServer() {
			fg.emitStreamingServerHandler(sd, md, svcName)
			continue
		}

		handlerName := fmt.Sprintf("_%s_%s_Handler", svcName, methodName)
		fmt.Fprintf(fg.body, "func %s(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {\n", handlerName)
		fmt.Fprintf(fg.body, "\tin := new(%s)\n", inType)
		fmt.Fprintf(fg.body, "\tif err := dec(in); err != nil {\n\t\treturn nil, err\n\t}\n")
		fmt.Fprintf(fg.body, "\tif interceptor == nil {\n")
		fmt.Fprintf(fg.body, "\t\treturn srv.(%sServer).%s(ctx, in)\n", svcName, methodName)
		fmt.Fprintf(fg.body, "\t}\n")
		fmt.Fprintf(fg.body, "\tinfo := &grpc.UnaryServerInfo{\n")
		fmt.Fprintf(fg.body, "\t\tServer:     srv,\n")
		fmt.Fprintf(fg.body, "\t\tFullMethod: \"/%s.%s/%s\",\n", sd.ParentFile().Package(), svcName, methodName)
		fmt.Fprintf(fg.body, "\t}\n")
		fmt.Fprintf(fg.body, "\thandler := func(ctx context.Context, req interface{}) (interface{}, error) {\n")
		fmt.Fprintf(fg.body, "\t\treturn srv.(%sServer).%s(ctx, req.(*%s))\n", svcName, methodName, inType)
		fmt.Fprintf(fg.body, "\t}\n")
		fmt.Fprintf(fg.body, "\treturn interceptor(ctx, in, info, handler)\n")
		fmt.Fprintf(fg.body, "}\n\n")
	}
}

func (fg *FileGenerator) emitGRPCServiceDesc(sd protoreflect.ServiceDescriptor, svcName string) {
	fmt.Fprintf(fg.body, "var _%s_serviceDesc = grpc.ServiceDesc{\n", svcName)
	fmt.Fprintf(fg.body, "\tServiceName: %q,\n", string(sd.FullName()))
	fmt.Fprintf(fg.body, "\tHandlerType: (*%sServer)(nil),\n", svcName)

	// Unary methods
	fmt.Fprintf(fg.body, "\tMethods: []grpc.MethodDesc{\n")
	for i := 0; i < sd.Methods().Len(); i++ {
		md := sd.Methods().Get(i)
		if md.IsStreamingServer() || md.IsStreamingClient() {
			continue
		}
		fmt.Fprintf(fg.body, "\t\t{\n")
		fmt.Fprintf(fg.body, "\t\t\tMethodName: %q,\n", string(md.Name()))
		fmt.Fprintf(fg.body, "\t\t\tHandler:    _%s_%s_Handler,\n", svcName, string(md.Name()))
		fmt.Fprintf(fg.body, "\t\t},\n")
	}
	fmt.Fprintf(fg.body, "\t},\n")

	// Streaming methods
	fmt.Fprintf(fg.body, "\tStreams: []grpc.StreamDesc{\n")
	for i := 0; i < sd.Methods().Len(); i++ {
		md := sd.Methods().Get(i)
		if !md.IsStreamingServer() && !md.IsStreamingClient() {
			continue
		}
		fmt.Fprintf(fg.body, "\t\t{\n")
		fmt.Fprintf(fg.body, "\t\t\tStreamName:    %q,\n", string(md.Name()))
		fmt.Fprintf(fg.body, "\t\t\tHandler:       _%s_%s_Handler,\n", svcName, string(md.Name()))
		fmt.Fprintf(fg.body, "\t\t\tServerStreams: %t,\n", md.IsStreamingServer())
		if md.IsStreamingClient() {
			fmt.Fprintf(fg.body, "\t\t\tClientStreams: true,\n")
		}
		fmt.Fprintf(fg.body, "\t\t},\n")
	}
	fmt.Fprintf(fg.body, "\t},\n")
	fmt.Fprintf(fg.body, "\tMetadata: %q,\n", fileName(string(sd.ParentFile().Path())))
	fmt.Fprintf(fg.body, "}\n\n")
}

// Streaming helpers

func (fg *FileGenerator) emitStreamingClientMethod(sd protoreflect.ServiceDescriptor, md protoreflect.MethodDescriptor, lowerSvcName, svcName, fullMethod string) {
	methodName := string(md.Name())
	inType := fg.grpcMessageType(md.Input())
	streamType := svcName + "_" + methodName + "Client"

	fmt.Fprintf(fg.body, "func (c *%sClient) %s(ctx context.Context, in *%s, opts ...grpc.CallOption) (%s, error) {\n",
		lowerSvcName, methodName, inType, streamType)
	fmt.Fprintf(fg.body, "\tstream, err := c.cc.NewStream(ctx, &_%s_serviceDesc.Streams[%d], %q, opts...)\n",
		svcName, fg.streamIndex(sd, md), fullMethod)
	fmt.Fprintf(fg.body, "\tif err != nil {\n\t\treturn nil, err\n\t}\n")
	fmt.Fprintf(fg.body, "\tx := &%s{stream}\n", toLowerFirst(streamType))
	fmt.Fprintf(fg.body, "\tif err := x.ClientStream.SendMsg(in); err != nil {\n\t\treturn nil, err\n\t}\n")
	fmt.Fprintf(fg.body, "\tif err := x.ClientStream.CloseSend(); err != nil {\n\t\treturn nil, err\n\t}\n")
	fmt.Fprintf(fg.body, "\treturn x, nil\n")
	fmt.Fprintf(fg.body, "}\n\n")

	// Stream interface
	outType := fg.grpcMessageType(md.Output())
	fmt.Fprintf(fg.body, "type %s interface {\n", streamType)
	fmt.Fprintf(fg.body, "\tRecv() (*%s, error)\n", outType)
	fmt.Fprintf(fg.body, "\tgrpc.ClientStream\n")
	fmt.Fprintf(fg.body, "}\n\n")

	fmt.Fprintf(fg.body, "type %s struct {\n", toLowerFirst(streamType))
	fmt.Fprintf(fg.body, "\tgrpc.ClientStream\n")
	fmt.Fprintf(fg.body, "}\n\n")

	fmt.Fprintf(fg.body, "func (x *%s) Recv() (*%s, error) {\n", toLowerFirst(streamType), outType)
	fmt.Fprintf(fg.body, "\tm := new(%s)\n", outType)
	fmt.Fprintf(fg.body, "\tif err := x.ClientStream.RecvMsg(m); err != nil {\n\t\treturn nil, err\n\t}\n")
	fmt.Fprintf(fg.body, "\treturn m, nil\n")
	fmt.Fprintf(fg.body, "}\n\n")
}

func (fg *FileGenerator) emitStreamingServerHandler(sd protoreflect.ServiceDescriptor, md protoreflect.MethodDescriptor, svcName string) {
	methodName := string(md.Name())
	inType := fg.grpcMessageType(md.Input())
	outType := fg.grpcMessageType(md.Output())
	streamType := svcName + "_" + methodName + "Server"

	handlerName := fmt.Sprintf("_%s_%s_Handler", svcName, methodName)
	fmt.Fprintf(fg.body, "func %s(srv interface{}, stream grpc.ServerStream) error {\n", handlerName)
	fmt.Fprintf(fg.body, "\tm := new(%s)\n", inType)
	fmt.Fprintf(fg.body, "\tif err := stream.RecvMsg(m); err != nil {\n\t\treturn err\n\t}\n")
	fmt.Fprintf(fg.body, "\treturn srv.(%sServer).%s(m, &%s{stream})\n", svcName, methodName, toLowerFirst(streamType))
	fmt.Fprintf(fg.body, "}\n\n")

	// Server stream interface
	fmt.Fprintf(fg.body, "type %s interface {\n", streamType)
	fmt.Fprintf(fg.body, "\tSend(*%s) error\n", outType)
	fmt.Fprintf(fg.body, "\tgrpc.ServerStream\n")
	fmt.Fprintf(fg.body, "}\n\n")

	fmt.Fprintf(fg.body, "type %s struct {\n", toLowerFirst(streamType))
	fmt.Fprintf(fg.body, "\tgrpc.ServerStream\n")
	fmt.Fprintf(fg.body, "}\n\n")

	fmt.Fprintf(fg.body, "func (x *%s) Send(m *%s) error {\n", toLowerFirst(streamType), outType)
	fmt.Fprintf(fg.body, "\treturn x.ServerStream.SendMsg(m)\n")
	fmt.Fprintf(fg.body, "}\n\n")
}

// Helper functions

func (fg *FileGenerator) grpcMessageType(md protoreflect.MessageDescriptor) string {
	msgPkg := string(md.ParentFile().Package())
	typeName := goMessageTypeName(md)
	if msgPkg == fg.imports.selfPkg {
		return typeName
	}
	alias := fg.imports.addProtoFileImport(md.ParentFile())
	return alias + "." + typeName
}

func (fg *FileGenerator) emitClientMethodSignature(md protoreflect.MethodDescriptor, indent string) {
	methodName := string(md.Name())
	inType := fg.grpcMessageType(md.Input())
	outType := fg.grpcMessageType(md.Output())
	svcName := string(md.Parent().(protoreflect.ServiceDescriptor).Name())

	if md.IsStreamingServer() {
		streamType := svcName + "_" + methodName + "Client"
		fmt.Fprintf(fg.body, "%s%s(ctx context.Context, in *%s, opts ...grpc.CallOption) (%s, error)", indent, methodName, inType, streamType)
	} else {
		fmt.Fprintf(fg.body, "%s%s(ctx context.Context, in *%s, opts ...grpc.CallOption) (*%s, error)", indent, methodName, inType, outType)
	}
}

func (fg *FileGenerator) emitServerMethodSignature(md protoreflect.MethodDescriptor, indent string) {
	methodName := string(md.Name())
	inType := fg.grpcMessageType(md.Input())
	outType := fg.grpcMessageType(md.Output())
	svcName := string(md.Parent().(protoreflect.ServiceDescriptor).Name())

	if md.IsStreamingServer() {
		streamType := svcName + "_" + methodName + "Server"
		fmt.Fprintf(fg.body, "%s%s(*%s, %s) error", indent, methodName, inType, streamType)
	} else {
		fmt.Fprintf(fg.body, "%s%s(context.Context, *%s) (*%s, error)", indent, methodName, inType, outType)
	}
}

func (fg *FileGenerator) streamIndex(sd protoreflect.ServiceDescriptor, target protoreflect.MethodDescriptor) int {
	idx := 0
	for i := 0; i < sd.Methods().Len(); i++ {
		md := sd.Methods().Get(i)
		if md == target {
			return idx
		}
		if md.IsStreamingServer() || md.IsStreamingClient() {
			idx++
		}
	}
	return idx
}

func toLowerFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

func fileName(path string) string {
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		return path[idx+1:]
	}
	return path
}
