package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"ride-sharing/services/trip-service/internal/infrastructure/events"
	"ride-sharing/services/trip-service/internal/infrastructure/grpc"
	"ride-sharing/services/trip-service/internal/infrastructure/repository"
	"ride-sharing/services/trip-service/internal/service"
	"ride-sharing/shared/env"
	"ride-sharing/shared/logger"
	"ride-sharing/shared/messaging"
	"ride-sharing/shared/tracing"
	"syscall"

	"go.uber.org/zap"
	grpcserver "google.golang.org/grpc"
)

var GrpcAddr = ":9093"

func main() {
	log := logger.Init("trip-service")
	defer log.Sync()
	rabbitMqURI := env.GetString("RABBITMQ_URI", "amqp://guest:guest@rabbitmq:5672/")

	inmemRepo := repository.NewInmemRepository()
	svc := service.NewService(inmemRepo)

	// Initialize Tracing
	tracerCfg := tracing.Config{
		ServiceName:    "trip-service",
		Environment:    env.GetString("ENVIRONMENT", "development"),
		JaegerEndpoint: env.GetString("JAEGER_ENDPOINT", "http://jaeger:4318"),
	}
	shutdownTracer, err := tracing.InitTracer(tracerCfg)
	if err != nil {
		log.Fatal("Failed to initialize tracer", zap.Error(err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer func() {
		if err := shutdownTracer(ctx); err != nil {
			log.Error("Failed to shutdown tracer", zap.Error(err))
		}
	}()

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		<-sigCh
		cancel()
	}()

	lis, err := net.Listen("tcp", GrpcAddr)
	if err != nil {
		log.Fatal("failed to listen", zap.Error(err))
	}

	// RabbitMQ connection
	rabbitmq, err := messaging.NewRabbitMQ(rabbitMqURI)
	if err != nil {
		log.Fatal("failed to connect RabbitMQ", zap.Error(err))
	}
	defer rabbitmq.Close()
	log.Info("RabbitMQ connected")

	// Start driver consumer
	driverConsumer := events.NewDriverConsumer(rabbitmq, svc)
	go func() {
		if err := driverConsumer.Start(ctx); err != nil {
			log.Fatal("Failed to start driver consumer", zap.Error(err))
		}
	}()

	// Start payment consumer
	paymentConsumer := events.NewPaymentConsumer(rabbitmq, svc)
	go func() {
		if err := paymentConsumer.Start(ctx); err != nil {
			log.Fatal("Failed to start payment consumer", zap.Error(err))
		}
	}()

	publisher := events.NewTripEventPublisher(rabbitmq)

	// Starting the gRPC server
	grpcServer := grpcserver.NewServer(tracing.WithTracingInterceptors()...)
	grpc.NewGRPCHandler(grpcServer, svc, publisher)

	log.Info("Starting gRPC server", zap.String("addr", lis.Addr().String()))

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			log.Error("failed to serve", zap.Error(err))
			cancel()
		}
	}()

	// Wait for the shutdown signal
	<-ctx.Done()
	log.Info("Shutting down server...")
	grpcServer.GracefulStop()
}
