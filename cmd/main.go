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
	"github.com/kartik7120/booking_broker-service/cmd/api/payment_service"
)

func main() {

	err := godotenv.Load()

	if err != nil {
		log.Fatal("Error loading .env file", err)
		os.Exit(1)
		return
	}

	redisClient := redis.NewClient(
		&redis.Options{
			Addr:     "localhost:6379",
			Password: "", // No password set
			DB:       0,  // Use default DB
			Protocol: 2,  // Connection protocol
		},
	)

	app := api.Config{
		Validator:   validator.New(),
		RedisClient: redisClient,
	}

	srv := &http.Server{
		Addr:    ":8080",
		Handler: app.Routes(),
	}

	// srv2 := &http.Server{
	// 	Addr:    ":8081",
	// 	Handler: app.Routes(),
	// }

	quit := make(chan os.Signal, 1)

	log.SetFormatter(&log.JSONFormatter{})
	log.SetOutput(os.Stdout)
	log.SetReportCaller(true)

	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	var opts []grpc.DialOption

	opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))

	conn, err := grpc.NewClient(":1102", opts...)

	if err != nil {
		log.Error("error connecting to client", err)
		os.Exit(1)
		return
	}

	conn2, err := grpc.NewClient(":1104", opts...)

	if err != nil {
		log.Error("error connecting to payment service", err)
		os.Exit(1)
		return
	}

	client := pb.NewMovieDBServiceClient(conn)

	paymentClient := payment_service.NewPaymentServiceClient(conn2)

	conn3, err := grpc.NewClient(":1101", opts...)

	if err != nil {
		log.Error("error connecting to auth service", err)
		os.Exit(1)
		return
	}

	app.MovieDB_service = client
	app.Payment_service = paymentClient
	app.Auth_Service = at.NewAuthServiceClient(conn3)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Error starting server: %v", err)
		}
	}()

	// go func() {
	// 	if err := srv2.ListenAndServeTLS("cert.pem", "key.pem"); err != nil && err != http.ErrServerClosed {
	// 		log.Fatalf("Error starting TLS server %v", err)
	// 	}
	// }()

	<-quit

	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Error shutting down server: %v", err)
	}

	log.Println("Server exiting")
}
