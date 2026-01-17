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
	rabbitmq_producer "github.com/kartik7120/booking_broker-service/cmd/api/payment_producer_service"
	ps "github.com/kartik7120/booking_broker-service/cmd/api/payment_service"
)

type Config struct {
	MovieDB_service          pb.MovieDBServiceClient
	Payment_service          ps.PaymentServiceClient
	Auth_Service             at.AuthServiceClient
	Validator                *validator.Validate
	RedisClient              *redis.Client
	Payment_Producer_service rabbitmq_producer.RabbitmqProducerServiceClient
}

func enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		w.Header().Set("Access-Control-Allow-Origin", "https://booking-front-end-198155959998.europe-west1.run.app")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (c *Config) Routes() http.Handler {
	mux := chi.NewRouter()

	mux.Use(cors.Handler(cors.Options{
		AllowedOrigins: []string{
			"https://booking-front-end-198155959998.europe-west1.run.app",
			"http://127.0.0.1:5173",
		},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	mux.Use(middleware.Heartbeat("/ping"))
	mux.Use(middleware.Logger)
	mux.Use(middleware.Recoverer)
	mux.Options("/*", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
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
	mux.Post("/validateOTP", c.ValidateOTP)
	mux.Post("/registerUser", c.RegisterUser)
	mux.Post("/loginUser", c.Login)
	mux.Get("/checkIfUserExists/{email}", c.CheckIfUserExists)
	mux.Get("/getVenue/{venueID}", c.GetVenue)
	mux.Get("/getMovieTimeSlot/{movieTimeSlotID}", c.GetMovieTimeSlot)
	mux.Post("/lockSeats", c.LockSeats)
	mux.Post("/payment-status", c.CheckPaymentStatus)
	mux.Get("/getTicketDetails/{ticketID}", c.GetTicketDetails)
	mux.Post("/webhook/strapihook", c.StapiWebhook)

	return mux
}
