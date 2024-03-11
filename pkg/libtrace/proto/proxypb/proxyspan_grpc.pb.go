// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.3.0
// - protoc             v4.25.2
// source: proto/proxyspan.proto

package proxypb

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

const (
	TraceProxyService_ExportTraceProxy_FullMethodName    = "/proto.TraceProxyService/ExportTraceProxy"
	TraceProxyService_ExportLogTraceProxy_FullMethodName = "/proto.TraceProxyService/ExportLogTraceProxy"
	TraceProxyService_Status_FullMethodName              = "/proto.TraceProxyService/Status"
)

// TraceProxyServiceClient is the client API for TraceProxyService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type TraceProxyServiceClient interface {
	// For performance reasons, it is recommended to keep this RPC
	// alive for the entire life of the application.
	ExportTraceProxy(ctx context.Context, in *ExportTraceProxyServiceRequest, opts ...grpc.CallOption) (*ExportTraceProxyServiceResponse, error)
	ExportLogTraceProxy(ctx context.Context, in *ExportLogTraceProxyServiceRequest, opts ...grpc.CallOption) (*ExportTraceProxyServiceResponse, error)
	Status(ctx context.Context, in *StatusRequest, opts ...grpc.CallOption) (*StatusResponse, error)
}

type traceProxyServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewTraceProxyServiceClient(cc grpc.ClientConnInterface) TraceProxyServiceClient {
	return &traceProxyServiceClient{cc}
}

func (c *traceProxyServiceClient) ExportTraceProxy(ctx context.Context, in *ExportTraceProxyServiceRequest, opts ...grpc.CallOption) (*ExportTraceProxyServiceResponse, error) {
	out := new(ExportTraceProxyServiceResponse)
	err := c.cc.Invoke(ctx, TraceProxyService_ExportTraceProxy_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *traceProxyServiceClient) ExportLogTraceProxy(ctx context.Context, in *ExportLogTraceProxyServiceRequest, opts ...grpc.CallOption) (*ExportTraceProxyServiceResponse, error) {
	out := new(ExportTraceProxyServiceResponse)
	err := c.cc.Invoke(ctx, TraceProxyService_ExportLogTraceProxy_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *traceProxyServiceClient) Status(ctx context.Context, in *StatusRequest, opts ...grpc.CallOption) (*StatusResponse, error) {
	out := new(StatusResponse)
	err := c.cc.Invoke(ctx, TraceProxyService_Status_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// TraceProxyServiceServer is the server API for TraceProxyService service.
// All implementations must embed UnimplementedTraceProxyServiceServer
// for forward compatibility
type TraceProxyServiceServer interface {
	// For performance reasons, it is recommended to keep this RPC
	// alive for the entire life of the application.
	ExportTraceProxy(context.Context, *ExportTraceProxyServiceRequest) (*ExportTraceProxyServiceResponse, error)
	ExportLogTraceProxy(context.Context, *ExportLogTraceProxyServiceRequest) (*ExportTraceProxyServiceResponse, error)
	Status(context.Context, *StatusRequest) (*StatusResponse, error)
	mustEmbedUnimplementedTraceProxyServiceServer()
}

// UnimplementedTraceProxyServiceServer must be embedded to have forward compatible implementations.
type UnimplementedTraceProxyServiceServer struct {
}

func (UnimplementedTraceProxyServiceServer) ExportTraceProxy(context.Context, *ExportTraceProxyServiceRequest) (*ExportTraceProxyServiceResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ExportTraceProxy not implemented")
}
func (UnimplementedTraceProxyServiceServer) ExportLogTraceProxy(context.Context, *ExportLogTraceProxyServiceRequest) (*ExportTraceProxyServiceResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ExportLogTraceProxy not implemented")
}
func (UnimplementedTraceProxyServiceServer) Status(context.Context, *StatusRequest) (*StatusResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Status not implemented")
}
func (UnimplementedTraceProxyServiceServer) mustEmbedUnimplementedTraceProxyServiceServer() {}

// UnsafeTraceProxyServiceServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to TraceProxyServiceServer will
// result in compilation errors.
type UnsafeTraceProxyServiceServer interface {
	mustEmbedUnimplementedTraceProxyServiceServer()
}

func RegisterTraceProxyServiceServer(s grpc.ServiceRegistrar, srv TraceProxyServiceServer) {
	s.RegisterService(&TraceProxyService_ServiceDesc, srv)
}

func _TraceProxyService_ExportTraceProxy_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ExportTraceProxyServiceRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TraceProxyServiceServer).ExportTraceProxy(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: TraceProxyService_ExportTraceProxy_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TraceProxyServiceServer).ExportTraceProxy(ctx, req.(*ExportTraceProxyServiceRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _TraceProxyService_ExportLogTraceProxy_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ExportLogTraceProxyServiceRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TraceProxyServiceServer).ExportLogTraceProxy(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: TraceProxyService_ExportLogTraceProxy_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TraceProxyServiceServer).ExportLogTraceProxy(ctx, req.(*ExportLogTraceProxyServiceRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _TraceProxyService_Status_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(StatusRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TraceProxyServiceServer).Status(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: TraceProxyService_Status_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TraceProxyServiceServer).Status(ctx, req.(*StatusRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// TraceProxyService_ServiceDesc is the grpc.ServiceDesc for TraceProxyService service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var TraceProxyService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "proto.TraceProxyService",
	HandlerType: (*TraceProxyServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "ExportTraceProxy",
			Handler:    _TraceProxyService_ExportTraceProxy_Handler,
		},
		{
			MethodName: "ExportLogTraceProxy",
			Handler:    _TraceProxyService_ExportLogTraceProxy_Handler,
		},
		{
			MethodName: "Status",
			Handler:    _TraceProxyService_Status_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "proto/proxyspan.proto",
}
