package main

import (
	"context"
	"crypto/tls"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/joho/godotenv"
	"github.com/kartik7120/booking_broker-service/cmd/api"
	authsvc "github.com/kartik7120/booking_broker-service/cmd/api/authService"
	pb "github.com/kartik7120/booking_broker-service/cmd/api/grpcClient"
	rabbitmqproducer "github.com/kartik7120/booking_broker-service/cmd/api/payment_producer_service"
	"github.com/kartik7120/booking_broker-service/cmd/api/payment_service"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

func main() {
	// --------------------------------------------------
	// ENV
	// --------------------------------------------------
	env := os.Getenv("ENV")
	if env == "" || env == "TEST" {
		_ = godotenv.Load()
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// --------------------------------------------------
	// Logging
	// --------------------------------------------------
	log.SetOutput(os.Stdout)
	log.SetFormatter(&log.JSONFormatter{})
	log.SetReportCaller(true)

	if env == "production" {
		log.SetLevel(log.InfoLevel)
	} else {
		log.SetLevel(log.DebugLevel)
	}

	// --------------------------------------------------
	// Redis
	// --------------------------------------------------
	redisOpts := &redis.Options{
		Addr:     "redis-12864.c232.us-east-1-2.ec2.cloud.redislabs.com:12864",
		Username: "default",
		Password: mustEnv("REDIS_PROD_PASS"),
		DB:       0,
	}

	if env == "TEST" {
		redisOpts = &redis.Options{
			Addr: "redis_booking_app:6379",
			DB:   0,
		}
	}

	redisClient := redis.NewClient(redisOpts)

	app := api.Config{
		Validator:   validator.New(),
		RedisClient: redisClient,
	}

	// --------------------------------------------------
	// gRPC CLIENTS
	// --------------------------------------------------

	// ✅ MovieDB (TLS :443)
	app.MovieDB_service = pb.NewMovieDBServiceClient(
		connectTLS(
			mustEnv("MOVIEDB_SERVICE_URL"),
			"booking-moviedb-service-198155959998.europe-west1.run.app",
		),
	)

	// ✅ Payment Service (PLAINTEXT :80 — h2c)
	app.Payment_service = payment_service.NewPaymentServiceClient(
		connectTLS(
			mustEnv("PAYMENT_SERVICE_URL"), // must be :80
			"payment-service-198155959998.europe-west1.run.app",
		),
	)

	// ✅ Auth Service (TLS :443)
	app.Auth_Service = authsvc.NewAuthServiceClient(
		connectTLS(
			mustEnv("AUTH_SERVICE_URL"),
			"auth-service-198155959998.europe-west1.run.app",
		),
	)

	// ✅ RabbitMQ Producer (TLS :443)
	app.Payment_Producer_service =
		rabbitmqproducer.NewRabbitmqProducerServiceClient(
			connectTLS(
				mustEnv("RABBITMQ_PRODUCER_SERVICE"),
				"rabbitmq-producer-service-198155959998.europe-west1.run.app",
			),
		)

	// --------------------------------------------------
	// HTTP SERVER
	// --------------------------------------------------
	srv := &http.Server{
		Addr: ":" + port,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			w.Header().Set("Access-Control-Allow-Origin", "https://booking-front-end-198155959998.europe-west1.run.app")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Credentials", "true")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			app.Routes().ServeHTTP(w, r)
		}),
	}

	go func() {
		log.Infof("Broker HTTP listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// --------------------------------------------------
	// SHUTDOWN
	// --------------------------------------------------
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down broker")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_ = srv.Shutdown(ctx)
	log.Info("Broker exited cleanly")
}

// --------------------------------------------------
// HELPERS
// --------------------------------------------------

func mustEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		log.Fatalf("Missing env var: %s", key)
	}
	return val
}

// TLS gRPC (Cloud Run :443)
func connectTLS(addr, serverName string) *grpc.ClientConn {
	conn, err := grpc.Dial(
		addr,
		grpc.WithTransportCredentials(
			credentials.NewTLS(&tls.Config{
				ServerName: serverName,
			}),
		),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second,
			Timeout:             10 * time.Second,
			PermitWithoutStream: true,
		}),
	)
	if err != nil {
		log.Fatalf("TLS gRPC connect failed (%s): %v", addr, err)
	}
	log.Infof("Connected (TLS): %s", addr)
	return conn
}

// PLAINTEXT gRPC (Cloud Run h2c :80)
func connectPlaintext(addr string) *grpc.ClientConn {
	conn, err := grpc.Dial(
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second,
			Timeout:             10 * time.Second,
			PermitWithoutStream: true,
		}),
	)
	if err != nil {
		log.Fatalf("Plaintext gRPC connect failed (%s): %v", addr, err)
	}
	log.Infof("Connected (PLAINTEXT): %s", addr)
	return conn
}
