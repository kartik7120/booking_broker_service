package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	pb "github.com/kartik7120/booking_broker-service/cmd/api/grpcClient"
)

func (c *Config) GetUpcomingMovies(w http.ResponseWriter, r *http.Request) {

	var requestBody struct {
		Longitude int16
		Latitude  int16
	}

	dateParam := chi.URLParam(r, "date")

	bodyBytes, err := io.ReadAll(r.Body)
	defer r.Body.Close()

	if err != nil {
		w.Write([]byte(string("error reading request body")))
	}

	err = json.Unmarshal(bodyBytes, &requestBody)

	if err != nil {
		w.Write([]byte("error unmarshalling json from request body"))
	}

	fmt.Printf("json body %#v", requestBody)

	response, err := c.MovieDB_service.GetUpcomingMovies(context.Background(), &pb.GetUpcomingMovieRequest{
		Date: dateParam,
	})

	if err != nil {
		w.Write([]byte("error getting upcoming movies"))
	}

	if response == nil {
		w.Write([]byte("response is nil"))
		return
	}

	if response.MovieList == nil {
		w.Write([]byte("response movies is nil"))
		return
	}

	jsonResponse, err := json.Marshal(response.MovieList)

	if err != nil {
		w.Write([]byte("error marshalling json response"))
	}

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(jsonResponse)

	if err != nil {
		w.Write([]byte("error writing json response"))
	}
}

func (c *Config) GetNowPlayingMovies(w http.ResponseWriter, r *http.Request) {
	var requestBody struct {
		Longitude int64
		Latitude  int64
	}

	bodyBytes, err := io.ReadAll(r.Body)
	defer r.Body.Close()

	if err != nil {
		w.Write([]byte(string("error reading request body")))
	}

	err = json.Unmarshal(bodyBytes, &requestBody)

	if err != nil {
		w.Write([]byte("error unmarshalling json from request body"))
	}

	response, err := c.MovieDB_service.GetNowPlayingMovies(context.Background(), &pb.GetNowPlayingMovieRequest{
		Longitude: requestBody.Longitude,
		Latitude:  requestBody.Latitude,
	})

	if err != nil {
		w.Write([]byte("error getting now playing movies"))
	}

	if response == nil {
		w.Write([]byte("response is nil"))
		return
	}

	if response.MovieList == nil {
		w.Write([]byte("response movies is nil"))
		return
	}

	jsonResponse, err := json.Marshal(response.MovieList)

	if err != nil {
		w.Write([]byte("error marshalling json response"))
	}

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(jsonResponse)

	if err != nil {
		w.Write([]byte("error writing json response"))
	}
}
