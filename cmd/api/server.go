package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	pb "github.com/kartik7120/booking_broker-service/cmd/api/grpcClient"
)

type Config struct {
	MovieDB_service pb.MovieDBServiceClient
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

	mux.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Welcome to the booking broker service"))
	})

	mux.Get("/getupcomingmovies/{date}", c.GetUpcomingMovies)
	mux.Get("/getnowplayingmovies", c.GetNowPlayingMovies)

	return mux
}
