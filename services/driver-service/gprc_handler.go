package main

import (
	"context"
	"ride-sharing/shared/logger"
	pb "ride-sharing/shared/proto/driver"
	"ride-sharing/shared/tracing"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	tracer = tracing.GetTracer("driver-service")
)

type driverGrpcHandler struct {
	pb.UnimplementedDriverServiceServer

	service *Service
}

func NewGrpcHandler(s *grpc.Server, service *Service) {
	handler := &driverGrpcHandler{
		service: service,
	}

	pb.RegisterDriverServiceServer(s, handler)
}

func (h *driverGrpcHandler) RegisterDriver(ctx context.Context, req *pb.RegisterDriverRequest) (*pb.RegisterDriverResponse, error) {
	logger.WithTrace(ctx).Info("[gRPC] RegisterDriver",
		zap.String("driver_id", req.GetDriverID()),
		zap.String("package", req.GetPackageSlug()),
	)

	driver, err := h.service.RegisterDriver(ctx, req.GetDriverID(), req.GetPackageSlug())
	if err != nil {
		logger.WithTrace(ctx).Error("failed to register driver", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to register driver")
	}

	return &pb.RegisterDriverResponse{
		Driver: driver,
	}, nil
}

func (h *driverGrpcHandler) UnregisterDriver(ctx context.Context, req *pb.RegisterDriverRequest) (*pb.RegisterDriverResponse, error) {
	logger.WithTrace(ctx).Info("[gRPC] UnregisterDriver", zap.String("driver_id", req.GetDriverID()))

	h.service.UnregisterDriver(req.GetDriverID())

	return &pb.RegisterDriverResponse{
		Driver: &pb.Driver{
			Id: req.GetDriverID(),
		},
	}, nil
}
