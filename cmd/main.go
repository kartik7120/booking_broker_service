package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/joho/godotenv"
	"github.com/kartik7120/booking_broker-service/cmd/api"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	at "github.com/kartik7120/booking_broker-service/cmd/api/authService"
	pb "github.com/kartik7120/booking_broker-service/cmd/api/grpcClient"
	rabbitmq_producer "github.com/kartik7120/booking_broker-service/cmd/api/payment_producer_service"
	"github.com/kartik7120/booking_broker-service/cmd/api/payment_service"
)

func main() {
	// Load env
	if err := godotenv.Load(); err != nil {
		log.Warn("No .env file found, continuing...")
	}

	// Redis client
	redisClient := redis.NewClient(&redis.Options{
		Addr:     "redis_booking_app:6379",
		Password: "",
		DB:       0,
	})

	app := api.Config{
		Validator:   validator.New(),
		RedisClient: redisClient,
	}

	// HTTP server
	srv := &http.Server{
		Addr:    ":8080",
		Handler: app.Routes(),
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	log.SetFormatter(&log.JSONFormatter{})
	log.SetOutput(os.Stdout)
	log.SetReportCaller(true)

	// gRPC options
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}

	// MovieDB service client
	movieConn, err := grpc.NewClient("booking_moviedb_service:1102", opts...)
	if err != nil {
		log.Fatalf("Error connecting to MovieDB service: %v", err)
	}
	defer movieConn.Close()
	app.MovieDB_service = pb.NewMovieDBServiceClient(movieConn)

	// Payment service client
	paymentConn, err := grpc.NewClient("booking_payment_service:1104", opts...)
	if err != nil {
		log.Fatalf("Error connecting to Payment service: %v", err)
	}
	defer paymentConn.Close()
	app.Payment_service = payment_service.NewPaymentServiceClient(paymentConn)

	// Auth service client
	authConn, err := grpc.NewClient("booking_auth_service:1101", opts...)
	if err != nil {
		log.Fatalf("Error connecting to Auth service: %v", err)
	}
	defer authConn.Close()
	app.Auth_Service = at.NewAuthServiceClient(authConn)

	// RabbitMQ producer service client
	rabbitConn, err := grpc.NewClient("rabbitmq_producer_service:1105", opts...)

	if err != nil {
		log.Fatalf("Error connecting to RabbitMQ producer service: %v", err)
	}

	defer rabbitConn.Close()

	app.Payment_Producer_service = rabbitmq_producer.NewRabbitmqProducerServiceClient(rabbitConn)

	// Start HTTP server
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Error starting server: %v", err)
		}
	}()

	<-quit
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Error shutting down server: %v", err)
	}

	log.Println("Server exited gracefully")
}
