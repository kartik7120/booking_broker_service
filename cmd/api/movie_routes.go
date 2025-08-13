package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	dodopayments "github.com/dodopayments/dodopayments-go"
	"github.com/go-chi/chi/v5"

	at "github.com/kartik7120/booking_broker-service/cmd/api/authService"
	pb "github.com/kartik7120/booking_broker-service/cmd/api/grpcClient"
	"github.com/kartik7120/booking_broker-service/cmd/api/payment_service"
	"github.com/kartik7120/booking_broker-service/cmd/api/utils"
)

func (c *Config) ValidateToken(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("Authorization")

	if token == "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "Missing Authorization header"}`, http.StatusUnauthorized)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	response, err := c.Auth_Service.ValidateToken(ctx, &at.ValdateTokenRequest{
		Token: token,
	})

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error validating token: %v"}`, err), http.StatusInternalServerError)
		return
	}

	if response == nil || response.Valid {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "Invalid token"}`, http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	jsonResponse, err := json.Marshal(map[string]string{
		"message": "Token is valid",
	})

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error marshalling JSON response: %v"}`, err), http.StatusInternalServerError)
		return
	}

	_, err = w.Write(jsonResponse)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error writing JSON response: %v"}`, err), http.StatusInternalServerError)
		return
	}
}

func (c *Config) Login(w http.ResponseWriter, r *http.Request) {

	var requestBody struct {
		Email    string `json:"email" validate:"required,email"`
		Password string `json:"password" validate:"required,min=8,max=32"`
	}

	bodyBytes, err := io.ReadAll(r.Body)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "error reading request body", http.StatusBadRequest)
		return
	}

	err = json.Unmarshal(bodyBytes, &requestBody)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "error unmarshalling request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err = c.Validator.Struct(requestBody); err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "error validating request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	response, err := c.Auth_Service.Login(ctx, &at.LoginUser{
		Email:    requestBody.Email,
		Password: requestBody.Password,
	})

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error logging in: %v"}`, err), http.StatusInternalServerError)
		return
	}

	if response == nil || response.Status != 200 || response.Error != "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error logging in: %s"}`, response.Error), http.StatusInternalServerError)
		return
	}

	cookie := &http.Cookie{
		Name:     "auth_token",
		Value:    response.Token,
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteStrictMode,
	}

	http.SetCookie(w, cookie)
	w.Header().Set("Content-Type", "application/json")

	w.WriteHeader(http.StatusOK)
	jsonResponse, err := json.Marshal(map[string]string{
		"message": "User logged in successfully",
		"token":   response.Token,
	})

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error marshalling JSON response: %v"}`, err), http.StatusInternalServerError)
		return
	}

	_, err = w.Write(jsonResponse)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error writing JSON response: %v"}`, err), http.StatusInternalServerError)
		return
	}
}

func (c *Config) RegisterUser(w http.ResponseWriter, r *http.Request) {

	var requestBody struct {
		Email       string `json:"email" validate:"required,email"`
		Password    string `json:"password" validate:"required,min=8,max=32"`
		PhoneNumber string `json:"phoneNumber" validate:"required,e164"` // assuming E.164 format
		Role        string `json:"role" validate:"required,oneof=admin user"`
	}

	bodyyBytes, err := io.ReadAll(r.Body)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "error reading request body", http.StatusBadRequest)
		return
	}

	err = json.Unmarshal(bodyyBytes, &requestBody)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "error unmarshalling request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err = c.Validator.Struct(requestBody); err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "error validating request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var role at.Role

	if requestBody.Role == "admin" {
		role = at.Role_ADMIN
	} else if requestBody.Role == "user" {
		role = at.Role_USER
	}

	response, err := c.Auth_Service.Resigter(ctx, &at.User{
		Email:    requestBody.Email,
		Password: requestBody.Password,
		Role:     role,
	})

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error registering user: %v"}`, err), http.StatusInternalServerError)
		return
	}

	if response == nil || response.Status != 200 || response.Error != "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error registering user: %s"}`, response.Error), http.StatusInternalServerError)
		return
	}

	cookie := &http.Cookie{
		Name:     "auth_token",
		Value:    response.Token,
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteStrictMode,
	}

	http.SetCookie(w, cookie)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	jsonResponse, err := json.Marshal(map[string]string{
		"message": "User registered successfully",
		"token":   response.Token,
	})

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error marshalling JSON response: %v"}`, err), http.StatusInternalServerError)
		return
	}

	_, err = w.Write(jsonResponse)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error writing JSON response: %v"}`, err), http.StatusInternalServerError)
		return
	}
}

func (c *Config) GetUpcomingMovies(w http.ResponseWriter, r *http.Request) {
	// Extract the "date" parameter from the URL
	dateParam := chi.URLParam(r, "date")

	fmt.Println("dateParam: ", dateParam)

	if dateParam == "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "Missing 'date' parameter in URL"}`, http.StatusBadRequest)
		return
	}

	// Call the gRPC service
	response, err := c.MovieDB_service.GetUpcomingMovies(context.Background(), &pb.GetUpcomingMovieRequest{
		Date: dateParam,
	})
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error getting upcoming movies: %v"}`, err), http.StatusInternalServerError)
		return
	}

	// Validate the gRPC response
	if response == nil || response.MovieList == nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "No upcoming movies found"}`, http.StatusNotFound)
		return
	}

	// Marshal the response to JSON
	jsonResponse, err := json.Marshal(response.MovieList)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error marshalling JSON response: %v"}`, err), http.StatusInternalServerError)
		return
	}

	// Write the JSON response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(jsonResponse)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error writing JSON response: %v"}`, err), http.StatusInternalServerError)
	}
}

func (c *Config) GetNowPlayingMovies(w http.ResponseWriter, r *http.Request) {
	var requestBody struct {
		Longitude float64 `json:"longitude"`
		Latitude  float64 `json:"latitude"`
	}

	// Read and parse the request body
	bodyBytes, err := io.ReadAll(r.Body)
	defer r.Body.Close()
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusBadRequest)
		return
	}

	err = json.Unmarshal(bodyBytes, &requestBody)

	if err != nil {
		http.Error(w, "error unmarshalling JSON from request body", http.StatusBadRequest)
		return
	}

	// Call the gRPC service
	response, err := c.MovieDB_service.GetNowPlayingMovies(context.Background(), &pb.GetNowPlayingMovieRequest{
		Longitude: int64(requestBody.Longitude),
		Latitude:  int64(requestBody.Latitude),
	})

	if err != nil {
		http.Error(w, "Error getting now playing movies from service", http.StatusInternalServerError)
		return
	}

	// Check if the response or MovieList is nil
	if response == nil || response.MovieList == nil {
		http.Error(w, "No movies found", http.StatusNotFound)
		return
	}

	// Marshal the response to JSON
	jsonResponse, err := json.Marshal(&response.MovieList)
	if err != nil {
		http.Error(w, "Error marshalling JSON response", http.StatusInternalServerError)
		return
	}

	fmt.Println("json response: ", jsonResponse)

	// Write the JSON response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(jsonResponse)
	if err != nil {
		http.Error(w, "Error writing JSON response", http.StatusInternalServerError)
	}
}

// https://www.gravatar.com/avatar/3b3be63a4c2a439b013787725dfce802?d=identicon

func (c *Config) GetMovieDetails(w http.ResponseWriter, r *http.Request) {
	// Extract the "id" parameter from the URL
	idParam := chi.URLParam(r, "id")
	if idParam == "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "Missing 'id' parameter in URL"}`, http.StatusBadRequest)
		return
	}

	// Call the gRPC service
	response, err := c.MovieDB_service.GetMovie(context.Background(), &pb.MovieRequest{
		Movieid: idParam,
	})
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error getting movie details: %v"}`, err), http.StatusInternalServerError)
		return
	}

	// Validate the gRPC response
	if response == nil || response.Movie == nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "No movie details found"}`, http.StatusNotFound)
		return
	}

	// Marshal the response to JSON
	jsonResponse, err := json.Marshal(response.Movie)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error marshalling JSON response: %v"}`, err), http.StatusInternalServerError)
		return
	}

	// Write the JSON response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(jsonResponse)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error writing JSON response: %v"}`, err), http.StatusInternalServerError)
	}
}

func (c *Config) GetMovieReviews(w http.ResponseWriter, r *http.Request) {
	var requestBody struct {
		Offset   int32       `json:"offset"`
		SortBy   pb.SortBy   `json:"sortBy"`
		Limit    int32       `json:"limit"`
		FilterBy pb.FilterBy `json:"filterBy"`
	}

	bodyBytes, err := io.ReadAll(r.Body)
	defer r.Body.Close()

	if err != nil {
		http.Error(w, "Error reading request body", http.StatusBadRequest)
		return
	}

	err = json.Unmarshal(bodyBytes, &requestBody)
	if err != nil {
		http.Error(w, "Error unmarshalling JSON from request body", http.StatusBadRequest)
		return
	}

	// Extract the "id" parameter from the URL
	id := chi.URLParam(r, "id")
	if id == "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "Missing 'id' parameter in URL"}`, http.StatusBadRequest)
		return
	}

	idInt, err := strconv.Atoi(id)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "Invalid 'id' parameter in URL"}`, http.StatusBadRequest)
		return
	}

	// Call the gRPC service

	response, err := c.MovieDB_service.GetAllMovieReviews(context.Background(), &pb.GetAllMovieReviewsRequest{
		MovieID:  int32(idInt),
		Offset:   requestBody.Offset,
		SortBy:   requestBody.SortBy,
		FilterBy: requestBody.FilterBy,
		Limit:    requestBody.Limit,
	})

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error getting movie reviews: %v"}`, err), http.StatusInternalServerError)
		return
	}

	// Validate the gRPC response

	if response == nil || response.ReviewList == nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "No movie reviews found"}`, http.StatusNotFound)
		return
	}

	// Marshal the response to JSON

	jsonResponse, err := json.Marshal(response)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error marshalling JSON response: %v"}`, err), http.StatusInternalServerError)
		return
	}

	// Write the JSON response

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(jsonResponse)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error writing JSON response: %v"}`, err), http.StatusInternalServerError)
	}

}

func (c *Config) AddMovieReview(w http.ResponseWriter, r *http.Request) {
	// Extract the "id" parameter from the URL
	var requestBody struct {
		UserID  int32  `json:"userId"`
		Title   string `json:"title"`
		Comment string `json:"comment"`
		Rating  int32  `json:"rating"`
	}

	bodyBytes, err := io.ReadAll(r.Body)
	defer r.Body.Close()

	if err != nil {
		http.Error(w, "Error reading request body", http.StatusBadRequest)
		return
	}

	err = json.Unmarshal(bodyBytes, &requestBody)

	if err != nil {
		http.Error(w, "Error unmarshalling JSON from request body", http.StatusBadRequest)
		return
	}

	if requestBody.Rating <= 0 || requestBody.Rating > 5 {
		http.Error(w, `error rating cannot be less than 1 or greater than 5`, http.StatusBadRequest)
		return
	}

	if requestBody.Comment == "" {
		http.Error(w, "error comment cannot be empty", http.StatusBadRequest)
		return
	}

	if requestBody.Title == "" {
		http.Error(w, "error title cannot be empty", http.StatusBadRequest)
		return
	}

	if requestBody.UserID <= 0 {
		http.Error(w, "error userId cannot be less than 1", http.StatusBadRequest)
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "Missing 'id' parameter in URL"}`, http.StatusBadRequest)
		return
	}

	idInt, err := strconv.Atoi(id)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "Invalid 'id' parameter in URL"}`, http.StatusBadRequest)
		return
	}

	response, err := c.MovieDB_service.AddReview(context.Background(), &pb.Review{
		MovieID: int32(idInt),
		UserID:  requestBody.UserID,
		Rating:  requestBody.Rating,
		Comment: requestBody.Comment,
		Title:   requestBody.Title,
	})

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error adding movie review: %v"}`, err), http.StatusInternalServerError)
		return
	}

	if response == nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "No movie review found"}`, http.StatusNotFound)
		return
	}

	jsonResponse, err := json.Marshal(response.Message)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error marshalling JSON response: %v"}`, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonResponse)
}

func (c *Config) DeleteMovieReview(w http.ResponseWriter, r *http.Request) {
	var requestBody struct {
		UserID   int32 `json:"userId"`
		ReviewID int32 `json:"reviewId"`
	}

	bodyBytes, err := io.ReadAll(r.Body)
	defer r.Body.Close()

	if err != nil {
		http.Error(w, "Error reading request body", http.StatusBadRequest)
		return
	}

	err = json.Unmarshal(bodyBytes, &requestBody)

	if err != nil {
		http.Error(w, "Error unmarshalling JSON from request body", http.StatusBadRequest)
		return
	}

	// Extract the "id" parameter from the URL

	id := chi.URLParam(r, "id")

	if id == "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "Missing 'id' parameter in URL"}`, http.StatusBadRequest)
		return
	}

	idInt, err := strconv.Atoi(id)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "Invalid 'id' parameter in URL"}`, http.StatusBadRequest)
		return
	}

	response, err := c.MovieDB_service.DeleteReview(context.Background(), &pb.ReviewRequest{
		MovieID:  int32(idInt),
		UserID:   requestBody.UserID,
		ReviewID: requestBody.ReviewID,
	})

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error deleting movie review: %v"}`, err), http.StatusInternalServerError)
		return
	}

	if response == nil || response.Review == nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "No movie review found"}`, http.StatusNotFound)
		return
	}

	jsonResponse, err := json.Marshal(response.Review)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error marshalling JSON response: %v"}`, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonResponse)
}

func (c *Config) GetMovieTimeSlots(w http.ResponseWriter, r *http.Request) {
	var requestBody struct {
		StartDate string  `json:"start_date"`
		EndDate   string  `json:"end_date"`
		MovieID   uint    `json:"movie_id"`
		Longitude float32 `json:"longitude"`
		Latitude  float32 `json:"latitude"`
	}

	bodyBytes, err := io.ReadAll(r.Body)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "error reading request body", 500)
		return
	}

	err = json.Unmarshal(bodyBytes, &requestBody)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "error ummarshalling request body : "+err.Error(), 500)
		return
	}

	if requestBody.StartDate == "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "start date cannot be empty", 400)
		return
	}

	if requestBody.EndDate == "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "end date cannot be empty", 400)
		return
	}

	response, err := c.MovieDB_service.GetMovieTimeSlots(context.Background(), &pb.GetMovieTimeSlotRequest{
		Movieid:   strconv.FormatUint(uint64(requestBody.MovieID), 10),
		StartDate: requestBody.StartDate,
		EndDate:   requestBody.EndDate,
		Longitude: requestBody.Longitude,
		Latitude:  requestBody.Latitude,
	})

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, err.Error(), 500)
		return
	}

	if response == nil || len(response.Venues) == 0 || len(response.MovieTimeSlots) == 0 {
		w.Header().Set("Content-Type", "application/json")
		if response == nil {
			http.Error(w, "response is nil", http.StatusNotFound)
			return
		}

		if len(response.Venues) == 0 {
			http.Error(w, "No venues could be found", http.StatusNotFound)
			return
		}

		if len(response.MovieTimeSlots) == 0 {
			http.Error(w, "No movie time slots could be found", http.StatusNotFound)
			return
		}
	}

	jsonResponse, err := json.Marshal(response)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error marshalling JSON response: %v"}`, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	_, err = w.Write(jsonResponse)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error writing JSON response: %v"}`, err), http.StatusInternalServerError)
	}

}

func (c *Config) GetSeatMatrix(w http.ResponseWriter, r *http.Request) {
	var requestBody struct {
		VenueID int32 `json:"venue_id"`
	}

	bodyBytes, err := io.ReadAll(r.Body)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "error reading request body", 500)
		return
	}

	err = json.Unmarshal(bodyBytes, &requestBody)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "error ummarshalling request body : "+err.Error(), 500)
		return
	}

	if requestBody.VenueID == 0 {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "venue id cannot be empty", 400)
		return
	}

	// if err != nil {
	// 	w.Header().Set("Content-Type", "application/json")
	// 	http.Error(w, "error converting venue id to int : "+err.Error(), 500)
	// 	return
	// }

	response, err := c.MovieDB_service.GetSeatMatrix(context.Background(), &pb.GetSeatMatrixRequest{
		Venueid: requestBody.VenueID,
	})

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, err.Error(), 500)
		return
	}

	if response == nil || len(response.Seats) == 0 {
		w.Header().Set("Content-Type", "application/json")
		if response == nil {
			http.Error(w, "response is nil", http.StatusNotFound)
			return
		}

		if len(response.Seats) == 0 {
			http.Error(w, "No seat matrix could be found", http.StatusNotFound)
			return
		}
	}

	jsonResponse, err := json.Marshal(response)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error marshalling JSON response: %v"}`, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(jsonResponse)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error writing JSON response: %v"}`, err), http.StatusInternalServerError)
	}

}

func (c *Config) BookSeats(w http.ResponseWriter, r *http.Request) {
	var requestBody *pb.BookSeatsRequest

	bodyBytes, err := io.ReadAll(r.Body)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "error reading request body", 500)
		return
	}

	err = json.Unmarshal(bodyBytes, &requestBody)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "error ummarshalling request body : "+err.Error(), 500)
		return
	}

	if len(requestBody.Seats) == 0 {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "booked seats cannot be empty", 400)
		return
	}

	response, err := c.MovieDB_service.BookSeats(context.Background(), &pb.BookSeatsRequest{
		Seats:           requestBody.Seats,
		MovieTimeSlotId: requestBody.MovieTimeSlotId,
		Email:           requestBody.Email,
		PhoneNumber:     requestBody.PhoneNumber,
	})

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, err.Error(), 500)
		return
	}

	if response == nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "response is nil", http.StatusNotFound)
		return
	}

	if response.Status != 200 {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, response.Message, int(response.Status))
		return
	}

	jsonResponse, err := json.Marshal(response)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error marshalling JSON response: %v"}`, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(jsonResponse)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error writing JSON response: %v"}`, err), http.StatusInternalServerError)
	}
}

func (c *Config) GetBookedSeats(w http.ResponseWriter, r *http.Request) {
	var requestBody pb.GetBookedSeatsRequest

	bodyBytes, err := io.ReadAll(r.Body)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "error reading request body", 500)
		return
	}

	err = json.Unmarshal(bodyBytes, &requestBody)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "error ummarshalling request body : "+err.Error(), 500)
		return
	}

	if requestBody.MovieTimeSlotId == 0 {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "movie time slot id cannot be empty", 400)
		return
	}

	response, err := c.MovieDB_service.GetBookedSeats(context.Background(), &requestBody)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, err.Error(), 500)
		return
	}

	if response == nil || len(response.BookedSeats) == 0 {
		w.Header().Set("Content-Type", "application/json")
		if response == nil {
			http.Error(w, "response is nil", http.StatusNotFound)
			return
		}

		if len(response.BookedSeats) == 0 {
			http.Error(w, "No booked seats could be found", http.StatusNotFound)
			return
		}
	}

	jsonResponse, err := json.Marshal(response)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error marshalling JSON response: %v"}`, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(jsonResponse)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error writing JSON response: %v"}`, err), http.StatusInternalServerError)
		return
	}

}

func (c *Config) HandleWebhookEvents(w http.ResponseWriter, r *http.Request) {

	var dummyResponse struct {
		IsValid bool
	}

	var requestBody dodopayments.WebhookEvent

	bodyBytes, err := io.ReadAll(r.Body)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"errors": "Error writing JSON response %v"}`, err), http.StatusInternalServerError)
		return
	}

	err = json.Unmarshal(bodyBytes, &requestBody)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"errors": "Error unmarshaling json %v"}`, err), http.StatusInternalServerError)
		return
	}

	fmt.Println(requestBody)

	// Need to call the create ticket function and book seats function
	// Need to call the send mail producer to send mail to user

	// switch requestBody.Type {
	// case "payment.succeeded":
	// 	// Commit seats and generate ticket for the customer
	// 	fmt.Println("Payment succeeded")
	// 	// response, err := c.MovieDB_service.BookSeats(context.Background(), &pb.BookSeatsRequest{})
	// case "payment.failed":
	// 	// re
	// }

	dummyResponse.IsValid = true

	jsonResponse, err := json.Marshal(dummyResponse)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	}
	_, err = w.Write(jsonResponse)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error writing JSON response: %v"}`, err), http.StatusInternalServerError)
		return
	}
}

func (c *Config) GetIdempotentKey(w http.ResponseWriter, r *http.Request) {

	// Generate a new idempotent key

	idempotentKey := utils.GenerateIdempotentKey()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := map[string]string{
		"idempotent_key": idempotentKey,
	}

	jsonResponse, err := json.Marshal(response)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error marshalling JSON response: %v"}`, err), http.StatusInternalServerError)
		return
	}

	_, err = w.Write(jsonResponse)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error writing JSON response: %v"}`, err), http.StatusInternalServerError)
		return
	}
}

func (c *Config) IsValidIdempotentKey(w http.ResponseWriter, r *http.Request) {

	var requestBody struct {
		Key string
	}

	bodyBytes, err := io.ReadAll(r.Body)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "Error occured while reading request body", http.StatusInternalServerError)
		return
	}

	err = json.Unmarshal(bodyBytes, &requestBody)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "error unmarshalling json", http.StatusInternalServerError)
		return
	}

	response, err := c.Payment_service.IsValidIdempotentKey(context.TODO(), &payment_service.IsValidIdempotentKeyRequest{
		IdempotentKey: requestBody.Key,
	})

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf("Error calling IsValidIdempotentKey rpc function: %v", err.Error()), http.StatusInternalServerError)
		return
	}

	if response.Error != "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf("Error calling IsValidIdempotentKey rpc function: %v", response.Error), http.StatusInternalServerError)
		return
	}

	if !response.IsValid {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "idempotent key is not valid", http.StatusBadRequest)
		return
	}

	var responseBody struct {
		IsValidKey bool
	}

	responseBody.IsValidKey = response.IsValid

	jsonResponse, err := json.Marshal(responseBody)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "error marshaling json response body", http.StatusInternalServerError)
		return
	}

	_, err = w.Write(jsonResponse)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "error writing response", http.StatusInternalServerError)
		return
	}
}

func (c *Config) CommitIdempotentKey(w http.ResponseWriter, r *http.Request) {

	var requestBody payment_service.CommitIdempotentKeyRequest

	bodyBytes, err := io.ReadAll(r.Body)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "error reading request body", 500)
		return
	}

	err = json.Unmarshal(bodyBytes, &requestBody)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "error unmarshalling request body : "+err.Error(), 500)
		return
	}

	if requestBody.IdempotentKey == "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "idempotent key cannot be empty", 400)
		return
	}

	response, err := c.Payment_service.CommitIdempotentKey(context.TODO(), &requestBody)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf("Error calling CommitIdempotentKey rpc function: %v", err.Error()), http.StatusInternalServerError)
		return
	}

	if response.Error != "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf("Error calling CommitIdempotentKey rpc function: %v", response.Error), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	responseBody := map[string]string{
		"message": "Idempotent key committed successfully",
	}

	jsonResponse, err := json.Marshal(responseBody)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error marshalling JSON response: %v"}`, err), http.StatusInternalServerError)
		return
	}

	_, err = w.Write(jsonResponse)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error writing JSON response: %v"}`, err), http.StatusInternalServerError)
		return
	}

}

func (c *Config) Create_Customer(w http.ResponseWriter, r *http.Request) {

	var requestBody payment_service.CreateCustomerRequest

	bodyBytes, err := io.ReadAll(r.Body)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "error reading request body", 500)
		return
	}

	err = json.Unmarshal(bodyBytes, &requestBody)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "error unmarshalling request body : "+err.Error(), 500)
		return
	}

	if requestBody.Email == "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "email cannot be empty", 400)
		return
	}

	response, err := c.Payment_service.CreateCustomer(context.TODO(), &requestBody)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf("Error calling CreateCustomer rpc function: %v", err.Error()), http.StatusInternalServerError)
		return
	}

	if response.Error != "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf("Error calling CreateCustomer rpc function: %v", response.Error), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	responseBody := map[string]string{
		"message":     "Customer created successfully",
		"customer_id": response.CustomerId,
	}

	jsonResponse, err := json.Marshal(responseBody)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error marshalling JSON response: %v"}`, err), http.StatusInternalServerError)
		return
	}

	_, err = w.Write(jsonResponse)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error writing JSON response: %v"}`, err), http.StatusInternalServerError)
		return
	}
}

func (c *Config) CreateOrder(w http.ResponseWriter, r *http.Request) {

	var requestBody payment_service.Create_Order_Request

	bodyBytes, err := io.ReadAll(r.Body)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "error reading request body", 500)
		return
	}

	err = json.Unmarshal(bodyBytes, &requestBody)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "error unmarshalling request body : "+err.Error(), 500)
		return
	}

	if requestBody.IdempotentKey == "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "idempotent key cannot be empty", 400)
		return
	}

	if requestBody.MovieTimeSlotId == 0 {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "movie time slot id cannot be empty", 400)
		return
	}

	if len(requestBody.SeatMatrixIDs) == 0 {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "seat matrix ids cannot be empty", 400)
		return
	}

	// if requestBody.CustomerId == 0 || requestBody.IdempotentKey == "" {
	// 	w.Header().Set("Content-Type", "application/json")
	// 	http.Error(w, "customer id and idempotent key cannot be empty", 400)
	// 	return
	// }

	response, err := c.Payment_service.CreateOrder(context.TODO(), &requestBody)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf("Error calling CreateOrder rpc function: %v", err.Error()), http.StatusInternalServerError)
		return
	}

	if response.Error != "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf("Error calling CreateOrder rpc function: %v", response.Error), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

}

func (c *Config) CreatePaymentLink(w http.ResponseWriter, r *http.Request) {

	var requestBody payment_service.CreatePaymentLinkRequest

	bodyBytes, err := io.ReadAll(r.Body)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "error reading request body", 500)
		return
	}

	err = json.Unmarshal(bodyBytes, &requestBody)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "error unmarshalling request body : "+err.Error(), 500)
		return
	}

	if requestBody.IdempotentKey == "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "order id cannot be empty", 400)
		return
	}

	response, err := c.Payment_service.GeneratePaymentLink(context.TODO(), &requestBody)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf("Error calling CreatePaymentLink rpc function: %v", err.Error()), http.StatusInternalServerError)
		return
	}

	if response.Error != "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf("Error calling CreatePaymentLink rpc function: %v", response.Error), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	jsonResponse, err := json.Marshal(response)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error marshalling JSON response: %v"}`, err), http.StatusInternalServerError)
		return
	}

	_, err = w.Write(jsonResponse)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error writing JSON response: %v"}`, err), http.StatusInternalServerError)
		return
	}
}
