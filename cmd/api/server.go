package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	redis "github.com/redis/go-redis/v9"

	validator "github.com/go-playground/validator/v10"
	at "github.com/kartik7120/booking_broker-service/cmd/api/authService"
	pb "github.com/kartik7120/booking_broker-service/cmd/api/grpcClient"
	ps "github.com/kartik7120/booking_broker-service/cmd/api/payment_service"
)

type Config struct {
	MovieDB_service pb.MovieDBServiceClient
	Payment_service ps.PaymentServiceClient
	Auth_Service    at.AuthServiceClient
	Validator       *validator.Validate
	RedisClient     *redis.Client
}

func (c *Config) Routes() http.Handler {
	mux := chi.NewRouter()

	mux.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"https://*", "http://*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	mux.Use(middleware.Heartbeat("/ping"))
	mux.Use(middleware.Logger)
	mux.Use(middleware.Recoverer)
	// mux.Use(utils.RedirectToHttpMiddleware)

	mux.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Welcome to the booking broker service"))
	})

	mux.Get("/getupcomingmovies/{date}", c.GetUpcomingMovies)
	mux.Post("/getnowplayingmovies", c.GetNowPlayingMovies)
	mux.Get("/getMovie/{id}", c.GetMovieDetails)
	mux.Post("/getAllMovieReview/{id}", c.GetMovieReviews)
	mux.Post("/addReview/{id}", c.AddMovieReview)
	mux.Post("/getMovieTimeSlots", c.GetMovieTimeSlots)
	mux.Post("/GetBookedSeats", c.GetBookedSeats)
	mux.Post("/BookSeats", c.BookSeats)
	mux.Post("/GetSeatMatrix", c.GetSeatMatrix)
	mux.Post("/webhook/events", c.HandleWebhookEvents)
	mux.Get("/getIdempotentKey", c.GetIdempotentKey)
	mux.Get("/isValidIdempotentKey", c.IsValidIdempotentKey)
	mux.Post("/commitIdempotentKey", c.CommitIdempotentKey)
	mux.Post("/createCustomer", c.Create_Customer)
	mux.Post("/createOrder", c.CreateOrder)
	mux.Post("/createPaymentLink", c.CreatePaymentLink)
	mux.Get("/validateToken", c.ValidateToken)
	mux.Post("/generateOTP", c.GenerateOTP)

	return mux
}
