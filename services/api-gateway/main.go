package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ride-sharing/shared/db"
	"ride-sharing/shared/env"
	"ride-sharing/shared/logger"
	"ride-sharing/shared/messaging"
	"ride-sharing/shared/middleware"
	"ride-sharing/shared/tracing"

	"go.uber.org/zap"
)

var (
	httpAddr    = env.GetString("HTTP_ADDR", ":8081")
	rabbitMqURI = env.GetString("RABBITMQ_URI", "amqp://guest:guest@rabbitmq:5672/")
)

func main() {
	log := logger.Init("api-gateway")
	defer log.Sync()
	log.Info("Starting API Gateway")

	// Initialize Tracing
	tracerCfg := tracing.Config{
		ServiceName:    "api-gateway",
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

	// RabbitMQ connection
	rabbitmq, err := messaging.NewRabbitMQ(rabbitMqURI)
	if err != nil {
		log.Fatal("Failed to connect RabbitMQ", zap.Error(err))
	}
	defer rabbitmq.Close()
	log.Info("RabbitMQ connected")

	// Redis connection
	redisURL := env.GetString("REDIS_URL", "")
	redisClient, err := db.NewRedisClient(redisURL)
	if err != nil {
		log.Fatal("Failed to connect Redis", zap.Error(err))
	}
	defer redisClient.Close()
	log.Info("Redis connected")

	mux := http.NewServeMux()

	mux.HandleFunc("POST /trip/preview", handleTripPreview)
	mux.HandleFunc("POST /trip/start", handleTripStart)
	mux.HandleFunc("/ws/drivers", func(w http.ResponseWriter, r *http.Request) {
		handleDriversWebSocket(w, r, rabbitmq)
	})
	mux.HandleFunc("/ws/riders", func(w http.ResponseWriter, r *http.Request) {
		handleRidersWebSocket(w, r, rabbitmq)
	})
	mux.HandleFunc("/webhook/stripe", func(w http.ResponseWriter, r *http.Request) {
		handleStripeWebhook(w, r, rabbitmq)
	})

	rateLimiter := middleware.NewRateLimiter(redisClient, 100, time.Minute, middleware.IPKey)
	globalMiddlewares := middleware.Chain(
		tracing.Middleware,
		middleware.EnableCORS,
		rateLimiter.Middleware,
		middleware.RequestLogger,
	)

	server := &http.Server{
		Addr:    httpAddr,
		Handler: globalMiddlewares(mux),
	}

	serverErrors := make(chan error, 1)

	go func() {
		log.Info("Server listening", zap.String("addr", httpAddr))
		serverErrors <- server.ListenAndServe()
	}()

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		log.Error("Error starting server", zap.Error(err))
	case sig := <-shutdown:
		log.Info("Shutting down", zap.String("signal", sig.String()))

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			log.Error("Graceful shutdown failed", zap.Error(err))

			if closeErr := server.Close(); closeErr != nil {
				log.Error("Could not force close the server", zap.Error(closeErr))
			}
		}
		log.Info("Server stopped gracefully")
	}
}
