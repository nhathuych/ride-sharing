package grpc

import (
	"context"
	"ride-sharing/services/trip-service/internal/domain"
	"ride-sharing/services/trip-service/internal/infrastructure/events"
	"ride-sharing/shared/logger"
	pb "ride-sharing/shared/proto/trip"
	"ride-sharing/shared/types"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type gRPCHandler struct {
	pb.UnimplementedTripServiceServer

	service   domain.TripService
	publisher *events.TripEventPublisher
}

func NewGRPCHandler(server *grpc.Server, service domain.TripService, publisher *events.TripEventPublisher) *gRPCHandler {
	handler := &gRPCHandler{
		service:   service,
		publisher: publisher,
	}

	pb.RegisterTripServiceServer(server, handler)
	return handler
}

func (h *gRPCHandler) CreateTrip(ctx context.Context, req *pb.CreateTripRequest) (*pb.CreateTripResponse, error) {
	logger.WithTrace(ctx).Info("CreateTrip request", zap.String("user_id", req.GetUserID()), zap.String("fare_id", req.GetRideFareID()))

	fareID := req.GetRideFareID()
	userID := req.GetUserID()

	// 1. Fetch & validate the fare
	rideFare, err := h.service.GetAndValidateFare(ctx, fareID, userID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to validate the fare : %v", err)
	}

	// 2. Call create trip
	trip, err := h.service.CreateTrip(ctx, rideFare)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create the trip : %v", err)
	}

	// 3. Add a comment at the end of the function to publish an event on the Async Comms module
	if err := h.publisher.PublishTripCreated(ctx, trip); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to publish the trip created event: %v", err)
	}

	logger.WithTrace(ctx).Info("trip created", zap.String("trip_id", trip.ID.Hex()))
	return &pb.CreateTripResponse{
		TripID: trip.ID.Hex(),
	}, nil
}

func (h *gRPCHandler) PreviewTrip(ctx context.Context, req *pb.PreviewTripRequest) (*pb.PreviewTripResponse, error) {
	logger.WithTrace(ctx).Info("PreviewTrip request", zap.String("user_id", req.GetUserID()))

	userID := req.GetUserID()
	pickup := req.GetStartLocation()
	destination := req.GetEndLocation()

	pickupCoord := &types.Coordinate{
		Latitude:  pickup.Latitude,
		Longitude: pickup.Longitude,
	}
	destinationCoord := &types.Coordinate{
		Latitude:  destination.Latitude,
		Longitude: destination.Longitude,
	}

	route, err := h.service.GetRoute(ctx, pickupCoord, destinationCoord)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get route: %v", err)
	}

	// 1. Estimate the ride fares prices based on the route (ex: distance)
	estimatedFares := h.service.EstimatePackagesPriceWithRoute(route)
	// 2. Store the ride fares for the create trip to fetch and validate
	fares, err := h.service.GenerateTripFares(ctx, estimatedFares, userID, route)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to generate the ride fares: %v", err)
	}

	return &pb.PreviewTripResponse{
		Route:     route.ToProto(),
		RideFares: domain.ToRideFaresProto(fares),
	}, nil
}
