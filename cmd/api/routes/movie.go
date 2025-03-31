package routes

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func GetUpcomingMovies(w http.ResponseWriter, r *http.Request) {
	dateParam := chi.URLParam(r, "date")

	w.Write([]byte("Getting upcoming movies, date param: " + dateParam))
}
