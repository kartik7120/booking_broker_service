package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	at "github.com/kartik7120/booking_broker-service/cmd/api/authService"
	rabbitmq_producer "github.com/kartik7120/booking_broker-service/cmd/api/payment_producer_service"
	strapitypes "github.com/kartik7120/booking_broker-service/cmd/api/strapi_types"

	pb "github.com/kartik7120/booking_broker-service/cmd/api/grpcClient"
	"github.com/kartik7120/booking_broker-service/cmd/api/payment_service"
	"github.com/kartik7120/booking_broker-service/cmd/api/utils"
	redis "github.com/redis/go-redis/v9"
)

type WebhookEvent struct {
	Type      string    `json:"type"`
	Timestamp string    `json:"timestamp"`
	Data      EventData `json:"data"`
}

type EventData struct {
	PaymentID          string    `json:"payment_id"`
	PaymentLink        string    `json:"payment_link"`
	Status             string    `json:"status"`
	Currency           string    `json:"currency"`
	TotalAmount        int64     `json:"total_amount"`
	SettlementAmount   int64     `json:"settlement_amount"`
	SettlementTax      int64     `json:"settlement_tax"`
	PaymentMethod      string    `json:"payment_method"`
	PaymentMethodType  string    `json:"payment_method_type"`
	CreatedAt          string    `json:"created_at"`
	UpdatedAt          *string   `json:"updated_at"`
	BusinessID         string    `json:"business_id"`
	BrandID            string    `json:"brand_id"`
	Customer           Customer  `json:"customer"`
	Billing            Billing   `json:"billing"`
	ProductCart        []Product `json:"product_cart"`
	Refunds            []Refund  `json:"refunds"`
	Disputes           []Dispute `json:"disputes"`
	Metadata           Metadata  `json:"metadata"`
	SubscriptionID     *string   `json:"subscription_id"`
	CheckoutSessionID  *string   `json:"checkout_session_id"`
	CardIssuingCountry *string   `json:"card_issuing_country"`
	CardLastFour       *string   `json:"card_last_four"`
	CardNetwork        *string   `json:"card_network"`
	CardType           *string   `json:"card_type"`
	DigitalProducts    bool      `json:"digital_products_delivered"`
	ErrorCode          *string   `json:"error_code"`
	ErrorMessage       *string   `json:"error_message"`
	DiscountID         *string   `json:"discount_id"`
	SettlementCurrency string    `json:"settlement_currency"`
	Tax                int64     `json:"tax"`
	PayloadType        string    `json:"payload_type"`
}

type Customer struct {
	CustomerID string `json:"customer_id"`
	Email      string `json:"email"`
	Name       string `json:"name"`
}

type Billing struct {
	City    string `json:"city"`
	State   string `json:"state"`
	Street  string `json:"street"`
	Zipcode string `json:"zipcode"`
	Country string `json:"country"`
}

type Product struct {
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
}

type Refund struct {
	// fill when refunds are supported
}

type Dispute struct {
	// fill when disputes are supported
}

type Metadata struct {
	BookedSeatsID   string `json:"booked_seats_id"`
	CustomerID      string `json:"customer_id"`
	CustomerPhone   string `json:"customer_phone"` // ✅ added phone number
	IdempotentKey   string `json:"idempotent_key"`
	MovieTimeSlotID string `json:"movie_time_slot_id"`
}

type RedisIdempotentValue struct {
	CustomerID      string   `json:"customer_id"`
	OrderIDs        []string `json:"order_ids"`
	BookedSeatsIDs  []int32  `json:"booked_seats_ids"`
	MovieTimeSlotID int32    `json:"movie_time_slot_id"`
	IsTicketSent    bool     `json:"is_ticket_sent"`
	IsMailSent      bool     `json:"is_mail_sent"`
	TicketID        string   `json:"ticket_ids"`
	PaymentStatus   string   `json:"payment_status"`
	ErrorMessage    string   `json:"error_message"`
}

type StrapiCastDTO struct {
	Name          string `json:"name"`
	CharacterName string `json:"character_name"`
	PhotoUrl      string `json:"photo_url"`
	MovieId       *int32 `json:"movieid"`
	Type          string `json:"type"`
	StarpiCastUid string `json:"starpi_cast_uid"`
	CastId        int32  `json:"cast_id"`
}

func (r RedisIdempotentValue) MarshalBinary() ([]byte, error) {
	return json.Marshal(r)
}

func (r *RedisIdempotentValue) UnmarshalBinary(data []byte) error {
	return json.Unmarshal(data, r)
}

func (c *Config) StapiWebhook(w http.ResponseWriter, r *http.Request) {
	fmt.Println("webhook triggered")

	bodyBytes, err := io.ReadAll(r.Body)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf("error reading request body: %s", err.Error()), http.StatusBadRequest)
		return
	}

	var webhookEvent strapitypes.StrapiWebHookType

	err = json.Unmarshal(bodyBytes, &webhookEvent)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf("error unmarshalling request body: %s", err.Error()), http.StatusBadRequest)
		return
	}

	fmt.Printf("webhook event: %+v\n", webhookEvent)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var cast rabbitmq_producer.Cast

	// Convert webhookEvent.Entry to rabbitmq_producer.Cast type

	entryJSON, _ := json.Marshal(webhookEvent.Entry)

	var dto StrapiCastDTO

	err = json.Unmarshal(entryJSON, &dto)

	fmt.Printf("strapi event %#v\n", dto)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf("error unmarshalling entry to StrapiCastDTO: %s", err.Error()), http.StatusBadRequest)
		fmt.Println("error unmarshalling entry to StrapiCastDTO: ", err)
		// TODO: Use the strapi API to fail the sync of this component
		return
	}

	cast.Name = dto.Name
	cast.CharacterName = dto.CharacterName
	cast.PhotoUrl = dto.PhotoUrl
	if strings.ToLower(dto.Type) == "actor" {
		cast.Type = rabbitmq_producer.CastType_ACTOR
	} else if strings.ToLower(dto.Type) == "director" {
		cast.Type = rabbitmq_producer.CastType_DIRECTOR
	} else if strings.ToLower(dto.Type) == "producer" {
		cast.Type = rabbitmq_producer.CastType_PRODUCER
	}

	if dto.MovieId != nil {
		cast.MovieId = *dto.MovieId
	} else {
		cast.MovieId = 0
	}

	cast.StarpiCastUidStr = dto.StarpiCastUid

	fmt.Printf("cast after type casting: %#v", cast)

	if webhookEvent.Event == "entry.publish" && webhookEvent.Model == "cast" {

		resp, err := c.Payment_Producer_service.Cast_Service_Producer(ctx, &cast)

		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, fmt.Sprintf("error calling Cast_Service_Producer: %s\n", err.Error()), http.StatusInternalServerError)
			// TODO: Use the strapi API to fail the sync of this component
			fmt.Println("error calling producer service inside strapi function : ", err.Error())
			return
		}

		if resp.Error != "" {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, fmt.Sprintf("error from Cast_Service_Producer: %s", resp.Error), http.StatusInternalServerError)
			fmt.Println("error from Cast_Service_Producer: ", strings.TrimSpace(resp.Error))
			// TODO: Use the strapi API to fail the sync of this component
			return
		}
	} else {
		fmt.Printf("other event encountered , need to implement the rest")
	}

}

func (c *Config) LockSeats(w http.ResponseWriter, r *http.Request) {

	var requestBody pb.GetBookedSeatsDetailsRequest

	bodyBytes, err := io.ReadAll(r.Body)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf("error reading request body: %s", err.Error()), http.StatusBadRequest)
		return
	}

	err = json.Unmarshal(bodyBytes, &requestBody)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf("error unmarshalling request body: %s", err.Error()), http.StatusBadRequest)
		return
	}

	response, err := c.MovieDB_service.LockBookedSeats(context.Background(), &requestBody)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf("error calling locked seats function: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	if response.Error != "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf("error calling locked seats function: %s", response.Error), http.StatusInternalServerError)
		return
	}

	var responseBody struct {
		LockedSeats []*pb.BookedSeats `json:"locked_seats"`
	}

	responseBody.LockedSeats = response.BookedSeats

	jsonResponse, err := json.Marshal(responseBody)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf("error marshalling json response: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	_, err = w.Write(jsonResponse)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf("error marshalling json response: %s", err.Error()), http.StatusInternalServerError)
		return
	}
}

func (c *Config) GetMovieTimeSlot(w http.ResponseWriter, r *http.Request) {
	// var requestBody struct {
	// 	MovieTimeSlotID int32 `json:"movie_time_slot_id"`
	// }

	// bodyBytes, err := io.ReadAll(r.Body)

	movieTimeSlotID := chi.URLParam(r, "movieTimeSlotID")

	if movieTimeSlotID == "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "movie time slot ID cannot be empty"}`, http.StatusBadRequest)
		return
	}

	movieTimeSlotInt, err := strconv.Atoi(movieTimeSlotID)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "error converting movie time slot to int: %s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	response, err := c.MovieDB_service.GetMovieTimeSlot(context.Background(), &pb.GetMovieTimeSlotDetailsRequest{
		MovieTimeSlotId: int32(movieTimeSlotInt),
	})

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "error calling GetMovieTimeSlot grpc function: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	var responseBody = &pb.MovieTimeSlot{}

	responseBody.Date = response.Date
	responseBody.Duration = response.Duration
	responseBody.EndTime = response.EndTime
	responseBody.StartTime = response.StartTime
	responseBody.MovieFormat = response.MovieFormat
	responseBody.MovieTimeSlotID = response.MovieTimeSlotID
	responseBody.Movieid = response.Movieid
	responseBody.Venueid = response.Venueid

	jsonResponse, err := json.Marshal(responseBody)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "error marhsalling json: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	_, err = w.Write(jsonResponse)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "error writing to response body: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
}

func (c *Config) GetVenue(w http.ResponseWriter, r *http.Request) {

	// var requestBody struct {
	// 	VenueID int32 `json:"venue_id"`
	// }

	// bodyBytes, err := io.ReadAll(r.Body)

	venueID := chi.URLParam(r, "venueID")

	if venueID == "" {
		w.Header().Set("Content-Type", "application/json")

		http.Error(w, `{"error": "venueID is empty"}`, http.StatusInternalServerError)
		return
	}

	// err = json.Unmarshal(bodyBytes, &requestBody)

	// if err != nil {
	// 	w.Header().Set("Content-Type", "application/json")

	// 	http.Error(w, fmt.Sprintf(`{"error": "error unmarshalling json: %v"}`, err.Error()), http.StatusInternalServerError)
	// 	return
	// }

	response, err := c.MovieDB_service.GetVenue(context.Background(), &pb.MovieRequest{
		Venueid: venueID,
	})

	if err != nil {
		w.Header().Set("Content-Type", "application/json")

		http.Error(w, fmt.Sprintf(`{"error": "error calling movie db service :%v"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	jsonResponse, err := json.Marshal(response.Venue)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "error marshalling json :%v"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	_, err = w.Write(jsonResponse)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "error writing response: %v"}`, err.Error()), http.StatusInternalServerError)
		return
	}
}

func (c *Config) CheckIfUserExists(w http.ResponseWriter, r *http.Request) {

	email := chi.URLParam(r, "email")

	var requestBody struct {
		Email string `json:"email" validate:"required,email"`
	}

	requestBody.Email = email

	// bodyBytes, err := io.ReadAll(r.Body)

	// if err != nil {

	// 	w.Header().Set("Content-Type", "application/json")
	// 	http.Error(w, "error reading request body", http.StatusInternalServerError)
	// 	return
	// }

	// err = json.Unmarshal(bodyBytes, &requestBody)

	// if err != nil {
	// 	w.Header().Set("Content-Type", "application/json")
	// 	http.Error(w, "error reading request body", http.StatusInternalServerError)
	// 	return
	// }

	if err := c.Validator.Struct(requestBody); err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "error validating input: "+err.Error(), http.StatusBadRequest)
		return
	}

	response, err := c.Auth_Service.CheckUserExists(context.Background(), &at.CheckUserExistsRequest{
		Email: requestBody.Email,
	})

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "error checking user exists: "+err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse, err := json.Marshal(response)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "error marshing response: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	_, err = w.Write(jsonResponse)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "error writing response: "+err.Error(), http.StatusInternalServerError)
		return
	}
}

func (c *Config) GenerateOTP(w http.ResponseWriter, r *http.Request) {

	// First retrieve the email from the request body
	// Genreate OTP
	// Insert data in the redis cache
	// Return the OTP to the user via email

	var requestBody struct {
		Email string `json:"email" validate:"required,email"`
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

	otp, err := utils.GenerateOTP(6)

	fmt.Println("Generated OTP: ", otp)

	if err != nil {
		fmt.Println("error generating OTP: ", err)
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error generating OTP: %v"}`, err), http.StatusInternalServerError)
		return
	}

	// Send OTP to the user via email

	templt := utils.SendOTPMailTemplate(requestBody.Email, otp)

	err = utils.SendMail(templt)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error sending OTP email: %v"}`, err), http.StatusInternalServerError)
		return
	}

	// Store the OTP in Redis with a 5-minute expiration time

	err = c.RedisClient.Set(ctx, requestBody.Email, otp, 5*time.Minute).Err()

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error storing OTP in Redis: %v"}`, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	w.WriteHeader(http.StatusOK)

	jsonResponse, err := json.Marshal(map[string]string{
		"message": "OTP sent successfully",
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

func (c *Config) ValidateOTP(w http.ResponseWriter, r *http.Request) {
	// Validate the OTP provided by the user
	// Check if the OTP exists in Redis
	// If it exists, delete it from Redis and return success
	// If it doesn't exist, return an error

	var requestBody struct {
		Email string `json:"email" validate:"required,email"`
		OTP   string `json:"otp" validate:"required,len=6"`
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

	storedOTP, err := c.RedisClient.Get(ctx, requestBody.Email).Result()

	if err == redis.Nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "OTP not found or expired"}`, http.StatusNotFound)
		return
	} else if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error retrieving OTP from Redis: %v"}`, err), http.StatusInternalServerError)
		return
	}

	if storedOTP != requestBody.OTP {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "Invalid OTP"}`, http.StatusUnauthorized)
		return
	}

	err = c.RedisClient.Del(ctx, requestBody.Email).Err()

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error deleting OTP from Redis: %v"}`, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	w.WriteHeader(http.StatusOK)

	jsonResponse, err := json.Marshal(map[string]string{
		"message": "OTP validated successfully",
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
		Email    string `json:"email" validate:"required,email"`
		Password string `json:"password" validate:"required,min=8,max=32"`
		// PhoneNumber string `json:"phoneNumber" validate:"required,e164"` // assuming E.164 format
		Role string `json:"role" validate:"required,oneof=admin user"`
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
		w.WriteHeader(http.StatusNoContent)
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
	w.Header().Set("Content-Type", "application/json")

	var requestBody struct {
		UserID          int32  `json:"userID"`
		Title           string `json:"title"`
		Comment         string `json:"comment"`
		Rating          int32  `json:"rating"`
		ReviewerName    string `json:"reviewerName"`
		ContainsSpoiler bool   `json:"containsSpoiler"`
		Language        string `json:"language"`
		Format          string `json:"format"`
	}

	// Read and parse request body
	bodyBytes, err := io.ReadAll(r.Body)
	defer r.Body.Close()
	if err != nil {
		http.Error(w, `{"error": "Failed to read request body"}`, http.StatusBadRequest)
		return
	}

	if err := json.Unmarshal(bodyBytes, &requestBody); err != nil {
		http.Error(w, `{"error": "Invalid JSON format"}`, http.StatusBadRequest)
		return
	}

	// Validate inputs
	if requestBody.Rating < 1 || requestBody.Rating > 5 {
		http.Error(w, `{"error": "Rating must be between 1 and 5"}`, http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(requestBody.Title) == "" {
		http.Error(w, `{"error": "Title cannot be empty"}`, http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(requestBody.Comment) == "" {
		http.Error(w, `{"error": "Comment cannot be empty"}`, http.StatusBadRequest)
		return
	}

	if requestBody.UserID != -1 && requestBody.UserID <= 0 {
		http.Error(w, `{"error": "Invalid userId"}`, http.StatusBadRequest)
		return
	}

	// Extract movie ID from URL
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, `{"error": "Missing 'id' parameter in URL"}`, http.StatusBadRequest)
		return
	}

	idInt, err := strconv.Atoi(id)
	if err != nil {
		http.Error(w, `{"error": "Invalid movie ID in URL"}`, http.StatusBadRequest)
		return
	}

	// gRPC request
	review := &pb.Review{
		MovieID:         int32(idInt),
		UserID:          requestBody.UserID,
		Title:           requestBody.Title,
		Comment:         requestBody.Comment,
		Rating:          requestBody.Rating,
		ReviewerName:    requestBody.ReviewerName,
		CreatedAt:       (time.Now().Unix()),
		ContainsSpoiler: requestBody.ContainsSpoiler,
		Language:        requestBody.Language,
		Format:          requestBody.Format,
	}

	response, err := c.MovieDB_service.AddReview(context.Background(), review)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Error adding review: %v"}`, err), http.StatusInternalServerError)
		return
	}

	if response == nil {
		http.Error(w, `{"error": "No response received from review service"}`, http.StatusInternalServerError)
		return
	}

	jsonResponse, err := json.Marshal(response)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Error encoding response: %v"}`, err), http.StatusInternalServerError)
		return
	}

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
			http.Error(w, `{"error": "response is nil"}`, http.StatusNotFound)
			return
		}

		if len(response.Venues) == 0 {
			http.Error(w, `{"error": "No venues could be found"}`, http.StatusNotFound)
			return
		}

		if len(response.MovieTimeSlots) == 0 {
			http.Error(w, `{"error": "No movie time slots could be found"}`, http.StatusNotFound)
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
			http.Error(w, `{"error": "No booked seats could be found"}`, http.StatusOK)
			return
		}
	}

	type BookedSeatResponse struct {
		ID              int32  `json:"id"`
		SeatNumber      string `json:"seat_number"`
		MovieTimeSlotID int32  `json:"movieTimeSlotID"`
		SeatMatrixID    int32  `json:"seatMatrixID"`
		IsBooked        bool   `json:"is_booked"`
	}

	var customResponse []BookedSeatResponse

	for _, v := range response.BookedSeats {

		customResponse = append(customResponse, BookedSeatResponse{
			ID:              v.Id,
			SeatNumber:      v.SeatNumber,
			MovieTimeSlotID: v.MovieTimeSlotID,
			SeatMatrixID:    v.SeatMatrixID,
			IsBooked:        v.IsBooked,
		})
	}

	jsonResponse, err := json.Marshal(customResponse)

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

func (c *Config) GetTicketDetails(w http.ResponseWriter, r *http.Request) {

	// var requestBody struct {
	// 	TicketID string `json:"ticket_id"`
	// }

	ticketID := chi.URLParam(r, "ticketID")

	// bodyBytes, err := io.ReadAll(r.Body)

	// if err != nil {
	// 	w.Header().Set("Content-Type", "application/json")
	// 	fmt.Println("error reading request body")
	// 	http.Error(w, fmt.Sprintf(`{"error":"error reading request body: %v"}`, err.Error()), http.StatusBadRequest)
	// 	return
	// }

	// err = json.Unmarshal(bodyBytes, &requestBody)

	// if err != nil {
	// 	w.Header().Set("Content-Type", "application/json")
	// 	fmt.Println("error unmarshalling json")
	// 	http.Error(w, fmt.Sprintf(`{"error":"error unmarshalling json: %v"}`, err.Error()), http.StatusBadRequest)
	// 	return
	// }

	response, err := c.MovieDB_service.GetTicketDetails(context.Background(), &pb.GetTicketDetailsRequest{
		TicketID: ticketID,
	})

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		fmt.Println("error calling get ticket details function")
		http.Error(w, fmt.Sprintf(`{"error":"error calling get ticket details function: %v"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	if response == nil {
		w.Header().Set("Content-Type", "application/json")
		fmt.Println("response of get ticket function is nil")
		http.Error(w, fmt.Sprintf(`{"error":"response of get ticket function is nil: %v"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	jsonResponse, err := json.Marshal(response)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")

		fmt.Println("error marshalling response json")
		http.Error(w, fmt.Sprintf(`{"error":"error marshalling response json: %v"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	_, err = w.Write(jsonResponse)

	if err != nil {
		fmt.Println("error sending response")
		http.Error(w, fmt.Sprintf(`{"error":"error sending response: %v"}`, err.Error()), http.StatusInternalServerError)
		return
	}
}

func (c *Config) HandleWebhookEvents(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "read body: %v"}`, err), http.StatusInternalServerError)
		return
	}

	var event WebhookEvent
	if err := json.Unmarshal(bodyBytes, &event); err != nil {
		fmt.Println(`error unmarshalling event: ` + err.Error())
		http.Error(w, fmt.Sprintf(`{"error": "unmarshal: %v"}`, err), http.StatusBadRequest)
		return
	}

	idKey := event.Data.Metadata.IdempotentKey
	fmt.Println("📩 Webhook received:", event.Type, "for", idKey)

	// common Redis object
	redisValue := RedisIdempotentValue{
		CustomerID:      event.Data.Metadata.CustomerID,
		MovieTimeSlotID: 0,
		IsTicketSent:    false,
		IsMailSent:      false,
		PaymentStatus:   "pending",
	}

	// parse movie slot id
	movieTimeSlotID, _ := strconv.Atoi(event.Data.Metadata.MovieTimeSlotID)
	redisValue.MovieTimeSlotID = int32(movieTimeSlotID)

	switch event.Type {
	case "payment.succeeded":
		fmt.Println("✅ Payment succeeded")

		fmt.Println("event object :", event)
		// parse booked seats
		var seatIDs []int32
		if err := json.Unmarshal([]byte(event.Data.Metadata.BookedSeatsID), &seatIDs); err != nil {
			fmt.Println("error parsing booked seats:", err)
			redisValue.PaymentStatus = "failed"
			redisValue.ErrorMessage = "invalid booked_seats_id format"
			break
		}

		seats := make([]*pb.BookedSeats, 0)
		for _, s := range seatIDs {
			seats = append(seats, &pb.BookedSeats{SeatMatrixID: s})
		}

		fmt.Printf("Calling the booked seats function %v", event)

		bookResp, err := c.MovieDB_service.BookSeats(context.Background(), &pb.BookSeatsRequest{
			Seats:           seats,
			MovieTimeSlotId: int32(movieTimeSlotID),
			Email:           event.Data.Customer.Email,
			PhoneNumber:     event.Data.Metadata.CustomerPhone,
		})

		fmt.Println("Finished calling booked seats function")

		if err != nil || bookResp.Status != 200 {
			redisValue.PaymentStatus = "failed"
			redisValue.ErrorMessage = fmt.Sprintf("booking error: %v", err)
			if err != nil {
				fmt.Println("error calling book seats function " + (err.Error()))
			}

			if bookResp.Status != 200 {
				fmt.Println("Book Seats function status not 200: ", bookResp.Message)
			}

			break
		}

		fmt.Println("Calling the ticket create function")
		// Create ticket
		ticketResp, err := c.MovieDB_service.CreateTicket(context.Background(), &pb.CreateTicketRequest{
			IdempotentKey: event.Data.Metadata.IdempotentKey,
			TrasactionId:  event.Data.PaymentID,
		})

		fmt.Println("Finished calling the create ticket endpoint")

		if err != nil || ticketResp.Status != 200 {
			redisValue.PaymentStatus = "failed"
			redisValue.ErrorMessage = fmt.Sprintf("ticket error: %v", err)

			fmt.Println("error calling create ticket function " + err.Error())
			break
		}

		fmt.Println("Calling the sent mail function")

		// send mail (optional, failures won’t block)
		err = utils.SendMailUsingMailtrap(event.Data.Customer.Email,
			"Your tickets are booked!",
			fmt.Sprintf("Ticket ID: %s", ticketResp.TicketID))

		if err != nil {
			fmt.Println("error sending the mail to the user", err.Error())
		} else {
			// success
			redisValue.IsTicketSent = true
			redisValue.PaymentStatus = "succeeded"
			redisValue.TicketID = ticketResp.TicketID
		}
		fmt.Println("FInished calling the mail trap function")

	case "payment.failed":
		fmt.Println("❌ Payment failed")
		redisValue.PaymentStatus = "failed"

	default:
		fmt.Println("ℹ️ Unknown event:", event.Type)
		redisValue.PaymentStatus = "ignored"
	}

	// persist in Redis (5 min or longer)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.RedisClient.Set(ctx, idKey, redisValue, 20*time.Minute).Err(); err != nil {
		fmt.Println("Redis set error:", err)
	}

	// always return 200 so gateway doesn’t retry
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

// TODO: Implement the webhook handler to listen to payment success and failure events
// TODO: On payment success, call the book seats function and create ticket function
// func (c *Config) HandleWebhookEvents(w http.ResponseWriter, r *http.Request) {

// 	// var dummyResponse struct {
// 	// 	IsValid bool
// 	// }

// 	var requestBody WebhookEvent

// 	bodyBytes, err := io.ReadAll(r.Body)

// 	if err != nil {
// 		w.Header().Set("Content-Type", "application/json")
// 		http.Error(w, fmt.Sprintf(`{"errors": "Error writing JSON response %v"}`, err), http.StatusInternalServerError)
// 		return
// 	}

// 	err = json.Unmarshal(bodyBytes, &requestBody)

// 	if err != nil {
// 		w.Header().Set("Content-Type", "application/json")
// 		http.Error(w, fmt.Sprintf(`{"errors": "Error unmarshaling json %v"}`, err), http.StatusInternalServerError)
// 		return
// 	}

// 	fmt.Println(requestBody)

// 	fmt.Println("Event Type: ", requestBody.Type)

// 	// Need to call the create ticket function and book seats function
// 	// Need to call the send mail producer to send mail to user

// 	// After calling the book seats function, we will get the booked seats ids
// 	// After calling the create ticket function, we will get the ticket ids

// 	// We will update the idempotent key in redis with the booked seats ids and ticket ids
// 	// We will also set the isTicketSent to true

// 	// If any of the functions fail, we will not update the idempotent key in redis
// 	// This way, if the webhook is called again, we can retry the operations

// 	// We will also need to handle the case where the payment fails
// 	// In this case, we will not call the book seats and create ticket functions

// 	// We can use the idempotent key to ensure that we do not process the same payment event multiple times

// 	// convert movietimeslotid to int32

// 	movieTimeSlotIDInt32, err := strconv.Atoi(requestBody.Data.Metadata.MovieTimeSlotID)

// 	if err != nil {
// 		w.Header().Set("Content-Type", "application/json")
// 		http.Error(w, fmt.Sprintf(`{"errors": "Error converting movie time slot id to int32 %v"}`, err), http.StatusInternalServerError)
// 		return
// 	}

// 	switch requestBody.Type {
// 	case "payment.succeeded":
// 		fmt.Println("✅ Payment succeeded")

// 		// Call the book seats function

// 		seatsToBeBooked := make([]*pb.BookedSeats, 0)

// 		for _, seatID := range requestBody.Data.Metadata.BookedSeatsID {

// 			seatIDInt, err := strconv.Atoi(seatID)

// 			if err != nil {
// 				w.Header().Set("Content-Type", "application/json")
// 				http.Error(w, fmt.Sprintf(`{"errors": "Error converting seat id to int32 %v"}`, err), http.StatusInternalServerError)
// 				return
// 			}

// 			seatsToBeBooked = append(seatsToBeBooked, &pb.BookedSeats{
// 				SeatMatrixID: int32(seatIDInt),
// 			})
// 		}

// 		response, err := c.MovieDB_service.BookSeats(context.Background(), &pb.BookSeatsRequest{
// 			Seats:           seatsToBeBooked,
// 			MovieTimeSlotId: int32(movieTimeSlotIDInt32),
// 			Email:           requestBody.Data.Customer.Email,
// 			PhoneNumber:     requestBody.Data.Metadata.CustomerPhone,
// 		})

// 		if err != nil {
// 			w.Header().Set("Content-Type", "application/json")

// 			http.Error(w, fmt.Sprintf(`{"errors": "Error booking seats %v"}`, err), http.StatusInternalServerError)

// 			// Log the error in redis against the idempotent key

// 			// So that we can retry the operation later

// 			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
// 			defer cancel()

// 			var redisValueObj RedisIdempotentValue

// 			c.RedisClient.Get(ctx, requestBody.Data.Metadata.IdempotentKey).Scan(&redisValueObj)

// 			redisValueObj.ErrorMessage = fmt.Sprintf("Error booking seats: %v", err)

// 			err = c.RedisClient.Set(ctx, requestBody.Data.Metadata.IdempotentKey, redisValueObj, 5*time.Minute).Err()

// 			if err != nil {
// 				fmt.Println("Error updating idempotent key in redis: ", err)
// 			}

// 			// End of logging the error in redis

// 			// We will return 200 OK to the payment gateway
// 			// So that it does not retry the webhook
// 			// We will handle the retry logic ourselves

// 			// http.Error(w, fmt.Sprintf(`{"errors": "Error booking seats %v"}`, err), http.StatusInternalServerError)
// 			return
// 		}

// 		if response == nil || response.Status != 200 {
// 			w.Header().Set("Content-Type", "application/json")
// 			http.Error(w, fmt.Sprintf(`{"errors": "Error booking seats %v"}`, "response is nil or status is not 200"), http.StatusInternalServerError)

// 			// Log the error in redis against the idempotent key

// 			// So that we can retry the operation later

// 			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
// 			defer cancel()

// 			var redisValueObj RedisIdempotentValue

// 			c.RedisClient.Get(ctx, requestBody.Data.Metadata.IdempotentKey).Scan(&redisValueObj)

// 			redisValueObj.ErrorMessage = "Error booking seats: response is nil or status is not 200"

// 			err = c.RedisClient.Set(ctx, requestBody.Data.Metadata.IdempotentKey, redisValueObj, 5*time.Minute).Err()

// 			if err != nil {
// 				fmt.Println("Error updating idempotent key in redis: ", err)
// 			}

// 			// End of logging the error in redis

// 			// We will return 200 OK to the payment gateway
// 			// So that it does not retry the webhook
// 			// We will handle the retry logic ourselves

// 			// http.Error(w, fmt.Sprintf(`{"errors": "Error booking seats %v"}`, "response is nil or status is not 200"), http.StatusInternalServerError)

// 			return
// 		}

// 		fmt.Println("Booked Seats Response: ", response)

// 		// Call the create ticket function

// 		createTicketResp, err := c.MovieDB_service.CreateTicket(context.Background(), &pb.CreateTicketRequest{
// 			IdempotentKey: requestBody.Data.Metadata.IdempotentKey,
// 			TrasactionId:  requestBody.Data.PaymentID,
// 		})

// 		if err != nil {
// 			w.Header().Set("Content-Type", "application/json")
// 			http.Error(w, fmt.Sprintf(`{"errors": "Error creating ticket %v"}`, err), http.StatusInternalServerError)

// 			// Log the error in redis against the idempotent key

// 			// So that we can retry the operation later

// 			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
// 			defer cancel()

// 			var redisValueObj RedisIdempotentValue

// 			c.RedisClient.Get(ctx, requestBody.Data.Metadata.IdempotentKey).Scan(&redisValueObj)

// 			redisValueObj.ErrorMessage = fmt.Sprintf("Error creating ticket: %v", err)

// 			err = c.RedisClient.Set(ctx, requestBody.Data.Metadata.IdempotentKey, redisValueObj, 5*time.Minute).Err()

// 			if err != nil {
// 				fmt.Println("Error updating idempotent key in redis: ", err)
// 			}

// 			// End of logging the error in redis

// 			// We will return 200 OK to the payment gateway
// 			// So that it does not retry the webhook
// 			// We will handle the retry logic ourselves

// 			// http.Error(w, fmt.Sprintf(`{"errors": "Error creating ticket %v"}`, err), http.StatusInternalServerError)
// 			return
// 		}

// 		if createTicketResp == nil || createTicketResp.Status != 200 {
// 			w.Header().Set("Content-Type", "application/json")
// 			http.Error(w, fmt.Sprintf(`{"errors": "Error creating ticket %v"}`, "response is nil or status is not 200"), http.StatusInternalServerError)

// 			// Log the error in redis against the idempotent key

// 			// So that we can retry the operation later

// 			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
// 			defer cancel()

// 			var redisValueObj RedisIdempotentValue

// 			c.RedisClient.Get(ctx, requestBody.Data.Metadata.IdempotentKey).Scan(&redisValueObj)

// 			redisValueObj.ErrorMessage = "Error creating ticket: response is nil or status is not 200"

// 			err = c.RedisClient.Set(ctx, requestBody.Data.Metadata.IdempotentKey, redisValueObj, 5*time.Minute).Err()

// 			if err != nil {
// 				fmt.Println("Error updating idempotent key in redis: ", err)
// 			}

// 			// End of logging the error in redis

// 			// We will return 200 OK to the payment gateway

// 			// So that it does not retry the webhook
// 			// We will handle the retry logic ourselves

// 			// http.Error(w, fmt.Sprintf(`{"errors": "Error creating ticket %v"}`, "response is nil or status is not 200"), http.StatusInternalServerError)
// 			return
// 		}

// 		fmt.Println("Create Ticket Response: ", createTicketResp)

// 		// Now, how do I redirect the user to the ticket page in frontend?
// 		// I think I can send the ticket ids in the response of book seats function
// 		// Then, the frontend can redirect the user to the ticket page

// 		// Update the idempotent key in redis with the booked seats ids and ticket ids
// 		// Set isTicketSent to true

// 		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
// 		defer cancel()

// 		redisValueObj := RedisIdempotentValue{
// 			CustomerID:      requestBody.Data.Metadata.CustomerID,
// 			MovieTimeSlotID: int32(movieTimeSlotIDInt32),
// 			IsTicketSent:    true,
// 			IsMailSent:      false,
// 			TicketID:        createTicketResp.TicketID,
// 			PaymentStatus:   "succeeded",
// 		}

// 		c.RedisClient.Get(ctx, requestBody.Data.Metadata.IdempotentKey).Scan(&redisValueObj)

// 		// Redirect the user to the ticket page in frontend with the ticket ids

// 		err = c.RedisClient.Set(ctx, requestBody.Data.Metadata.IdempotentKey, redisValueObj, 5*time.Minute).Err()

// 		if err != nil {
// 			w.Header().Set("Content-Type", "application/json")
// 			http.Error(w, fmt.Sprintf(`{"errors": "Error updating idempotent key in redis %v"}`, err), http.StatusInternalServerError)
// 			return
// 		}

// 		// How to redirect the user to the ticket page in frontend from here?

// 		fmt.Println("User can be redirected to ticket page with ticket ids: ", createTicketResp.TicketID)

// 		err = utils.SendMailUsingMailtrap(requestBody.Data.Customer.Email, `Your tickets are booked!`, fmt.Sprintf("Your tickets are booked! Your ticket ids are: %v", createTicketResp.TicketID))

// 		if err != nil {
// 			fmt.Println("Error sending mail: ", err)

// 			// We will not return error to payment gateway
// 			// We will log the error in redis against the idempotent key

// 			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
// 			defer cancel()

// 			var redisValueObj RedisIdempotentValue

// 			c.RedisClient.Get(ctx, requestBody.Data.Metadata.IdempotentKey).Scan(&redisValueObj)

// 			redisValueObj.ErrorMessage = fmt.Sprintf("Error sending mail: %v", err)

// 			err = c.RedisClient.Set(ctx, requestBody.Data.Metadata.IdempotentKey, redisValueObj, 5*time.Minute).Err()

// 			if err != nil {
// 				fmt.Println("Error updating idempotent key in redis: ", err)
// 			}

// 			// End of logging the error in redis

// 			// We will return 200 OK to the payment gateway
// 			// So that it does not retry the webhook
// 			// We will handle the retry logic ourselves

// 			// http.Error(w, fmt.Sprintf(`{"errors": "Error sending mail %v"}`, err), http.StatusInternalServerError)
// 			return
// 		}

// 		// Maybe I can send the ticket ids in the response of this webhook handler
// 		// Then, the frontend can redirect the user to the ticket page

// 		// But, how will the frontend know that the payment is successful and it needs to call this webhook handler?

// 		// I think the frontend can poll an endpoint to check if the payment is successful
// 		// Once the payment is successful, the frontend can call this webhook handler
// 		// The frontend can get the ticket ids from the response of this webhook handler
// 		// Then, the frontend can redirect the user to the ticket page

// 	case "payment.failed":
// 		fmt.Println("❌ Payment failed")

// 		// Set payment failed in redis

// 		redisValueObj := RedisIdempotentValue{
// 			CustomerID:      requestBody.Data.Metadata.CustomerID,
// 			MovieTimeSlotID: int32(movieTimeSlotIDInt32),
// 			IsTicketSent:    false,
// 			IsMailSent:      false,
// 			PaymentStatus:   "failed",
// 		}

// 		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
// 		defer cancel()

// 		c.RedisClient.Get(ctx, requestBody.Data.Metadata.IdempotentKey).Scan(&redisValueObj)

// 		redisValueObj.ErrorMessage = fmt.Sprintf("Error received from payment: %v", err)

// 		err = c.RedisClient.Set(ctx, requestBody.Data.Metadata.IdempotentKey, redisValueObj, 5*time.Minute).Err()

// 		if err != nil {
// 			fmt.Println("error setting payment failiure event")
// 			return
// 		}

// 	default:
// 		fmt.Println("ℹ️ Unknown event:", requestBody.Type)
// 	}

// 	// dummyResponse.IsValid = true

// 	// jsonResponse, err := json.Marshal(dummyResponse)

// 	// if err != nil {
// 	// 	w.Header().Set("Content-Type", "application/json")
// 	// 	w.WriteHeader(http.StatusOK)
// 	// }
// 	// _, err = w.Write(jsonResponse)

// 	// if err != nil {
// 	// 	w.Header().Set("Content-Type", "application/json")
// 	// 	http.Error(w, fmt.Sprintf(`{"error": "Error writing JSON response: %v"}`, err), http.StatusInternalServerError)
// 	// 	return
// 	// }
// }

// func (c *Config) HandleWebhookEvents(w http.ResponseWriter, r *http.Request) {

// 	// var dummyResponse struct {
// 	// 	IsValid bool
// 	// }

// 	var requestBody WebhookEvent

// 	bodyBytes, err := io.ReadAll(r.Body)

// 	if err != nil {
// 		w.Header().Set("Content-Type", "application/json")
// 		http.Error(w, fmt.Sprintf(`{"errors": "Error writing JSON response %v"}`, err), http.StatusInternalServerError)
// 		return
// 	}

// 	err = json.Unmarshal(bodyBytes, &requestBody)

// 	if err != nil {
// 		w.Header().Set("Content-Type", "application/json")
// 		http.Error(w, fmt.Sprintf(`{"errors": "Error unmarshaling json %v"}`, err), http.StatusInternalServerError)
// 		return
// 	}

// 	fmt.Println(requestBody)

// 	fmt.Println("Event Type: ", requestBody.Type)

// 	// Need to call the create ticket function and book seats function
// 	// Need to call the send mail producer to send mail to user

// 	// After calling the book seats function, we will get the booked seats ids
// 	// After calling the create ticket function, we will get the ticket ids

// 	// We will update the idempotent key in redis with the booked seats ids and ticket ids
// 	// We will also set the isTicketSent to true

// 	// If any of the functions fail, we will not update the idempotent key in redis
// 	// This way, if the webhook is called again, we can retry the operations

// 	// We will also need to handle the case where the payment fails
// 	// In this case, we will not call the book seats and create ticket functions

// 	// We can use the idempotent key to ensure that we do not process the same payment event multiple times

// 	// convert movietimeslotid to int32

// 	movieTimeSlotIDInt32, err := strconv.Atoi(requestBody.Data.Metadata.MovieTimeSlotID)

// 	if err != nil {
// 		w.Header().Set("Content-Type", "application/json")
// 		http.Error(w, fmt.Sprintf(`{"errors": "Error converting movie time slot id to int32 %v"}`, err), http.StatusInternalServerError)
// 		return
// 	}

// 	switch requestBody.Type {
// 	case "payment.succeeded":
// 		fmt.Println("✅ Payment succeeded")

// 		// Call the book seats function

// 		seatsToBeBooked := make([]*pb.BookedSeats, 0)

// 		for _, seatID := range requestBody.Data.Metadata.BookedSeatsID {

// 			seatIDInt, err := strconv.Atoi(seatID)

// 			if err != nil {
// 				w.Header().Set("Content-Type", "application/json")
// 				http.Error(w, fmt.Sprintf(`{"errors": "Error converting seat id to int32 %v"}`, err), http.StatusInternalServerError)
// 				return
// 			}

// 			seatsToBeBooked = append(seatsToBeBooked, &pb.BookedSeats{
// 				SeatMatrixID: int32(seatIDInt),
// 			})
// 		}

// 		response, err := c.MovieDB_service.BookSeats(context.Background(), &pb.BookSeatsRequest{
// 			Seats:           seatsToBeBooked,
// 			MovieTimeSlotId: int32(movieTimeSlotIDInt32),
// 			Email:           requestBody.Data.Customer.Email,
// 			PhoneNumber:     requestBody.Data.Metadata.CustomerPhone,
// 		})

// 		if err != nil {
// 			w.Header().Set("Content-Type", "application/json")

// 			http.Error(w, fmt.Sprintf(`{"errors": "Error booking seats %v"}`, err), http.StatusInternalServerError)

// 			// Log the error in redis against the idempotent key

// 			// So that we can retry the operation later

// 			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
// 			defer cancel()

// 			var redisValueObj RedisIdempotentValue

// 			c.RedisClient.Get(ctx, requestBody.Data.Metadata.IdempotentKey).Scan(&redisValueObj)

// 			redisValueObj.ErrorMessage = fmt.Sprintf("Error booking seats: %v", err)

// 			err = c.RedisClient.Set(ctx, requestBody.Data.Metadata.IdempotentKey, redisValueObj, 5*time.Minute).Err()

// 			if err != nil {
// 				fmt.Println("Error updating idempotent key in redis: ", err)
// 			}

// 			// End of logging the error in redis

// 			// We will return 200 OK to the payment gateway
// 			// So that it does not retry the webhook
// 			// We will handle the retry logic ourselves

// 			// http.Error(w, fmt.Sprintf(`{"errors": "Error booking seats %v"}`, err), http.StatusInternalServerError)
// 			return

// 		}

// 		if response == nil || response.Status != 200 {
// 			w.Header().Set("Content-Type", "application/json")
// 			http.Error(w, fmt.Sprintf(`{"errors": "Error booking seats %v"}`, "response is nil or status is not 200"), http.StatusInternalServerError)

// 			// Log the error in redis against the idempotent key

// 			// So that we can retry the operation later

// 			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
// 			defer cancel()

// 			var redisValueObj RedisIdempotentValue

// 			c.RedisClient.Get(ctx, requestBody.Data.Metadata.IdempotentKey).Scan(&redisValueObj)

// 			redisValueObj.ErrorMessage = "Error booking seats: response is nil or status is not 200"

// 			err = c.RedisClient.Set(ctx, requestBody.Data.Metadata.IdempotentKey, redisValueObj, 5*time.Minute).Err()

// 			if err != nil {
// 				fmt.Println("Error updating idempotent key in redis: ", err)
// 			}

// 			// End of logging the error in redis

// 			// We will return 200 OK to the payment gateway
// 			// So that it does not retry the webhook
// 			// We will handle the retry logic ourselves

// 			// http.Error(w, fmt.Sprintf(`{"errors": "Error booking seats %v"}`, "response is nil or status is not 200"), http.StatusInternalServerError)

// 			return
// 		}

// 		fmt.Println("Booked Seats Response: ", response)

// 		// Call the create ticket function

// 		createTicketResp, err := c.MovieDB_service.CreateTicket(context.Background(), &pb.CreateTicketRequest{
// 			IdempotentKey: requestBody.Data.Metadata.IdempotentKey,
// 			TrasactionId:  requestBody.Data.PaymentID,
// 		})

// 		if err != nil {
// 			w.Header().Set("Content-Type", "application/json")
// 			http.Error(w, fmt.Sprintf(`{"errors": "Error creating ticket %v"}`, err), http.StatusInternalServerError)

// 			// Log the error in redis against the idempotent key

// 			// So that we can retry the operation later

// 			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
// 			defer cancel()

// 			var redisValueObj RedisIdempotentValue

// 			c.RedisClient.Get(ctx, requestBody.Data.Metadata.IdempotentKey).Scan(&redisValueObj)

// 			redisValueObj.ErrorMessage = fmt.Sprintf("Error creating ticket: %v", err)

// 			err = c.RedisClient.Set(ctx, requestBody.Data.Metadata.IdempotentKey, redisValueObj, 5*time.Minute).Err()

// 			if err != nil {
// 				fmt.Println("Error updating idempotent key in redis: ", err)
// 			}

// 			// End of logging the error in redis

// 			// We will return 200 OK to the payment gateway
// 			// So that it does not retry the webhook
// 			// We will handle the retry logic ourselves

// 			// http.Error(w, fmt.Sprintf(`{"errors": "Error creating ticket %v"}`, err), http.StatusInternalServerError)
// 			return
// 		}

// 		if createTicketResp == nil || createTicketResp.Status != 200 {
// 			w.Header().Set("Content-Type", "application/json")
// 			http.Error(w, fmt.Sprintf(`{"errors": "Error creating ticket %v"}`, "response is nil or status is not 200"), http.StatusInternalServerError)

// 			// Log the error in redis against the idempotent key

// 			// So that we can retry the operation later

// 			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
// 			defer cancel()

// 			var redisValueObj RedisIdempotentValue

// 			c.RedisClient.Get(ctx, requestBody.Data.Metadata.IdempotentKey).Scan(&redisValueObj)

// 			redisValueObj.ErrorMessage = "Error creating ticket: response is nil or status is not 200"

// 			err = c.RedisClient.Set(ctx, requestBody.Data.Metadata.IdempotentKey, redisValueObj, 5*time.Minute).Err()

// 			if err != nil {
// 				fmt.Println("Error updating idempotent key in redis: ", err)
// 			}

// 			// End of logging the error in redis

// 			// We will return 200 OK to the payment gateway

// 			// So that it does not retry the webhook
// 			// We will handle the retry logic ourselves

// 			// http.Error(w, fmt.Sprintf(`{"errors": "Error creating ticket %v"}`, "response is nil or status is not 200"), http.StatusInternalServerError)
// 			return
// 		}

// 		fmt.Println("Create Ticket Response: ", createTicketResp)

// 		// Now, how do I redirect the user to the ticket page in frontend?
// 		// I think I can send the ticket ids in the response of book seats function
// 		// Then, the frontend can redirect the user to the ticket page

// 		// Update the idempotent key in redis with the booked seats ids and ticket ids
// 		// Set isTicketSent to true

// 		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
// 		defer cancel()

// 		redisValueObj := RedisIdempotentValue{
// 			CustomerID:      requestBody.Data.Metadata.CustomerID,
// 			MovieTimeSlotID: int32(movieTimeSlotIDInt32),
// 			IsTicketSent:    true,
// 			IsMailSent:      false,
// 			TicketID:        createTicketResp.TicketID,
// 			PaymentStatus:   "succeeded",
// 		}

// 		c.RedisClient.Get(ctx, requestBody.Data.Metadata.IdempotentKey).Scan(&redisValueObj)

// 		// Redirect the user to the ticket page in frontend with the ticket ids

// 		err = c.RedisClient.Set(ctx, requestBody.Data.Metadata.IdempotentKey, redisValueObj, 5*time.Minute).Err()

// 		if err != nil {
// 			w.Header().Set("Content-Type", "application/json")
// 			http.Error(w, fmt.Sprintf(`{"errors": "Error updating idempotent key in redis %v"}`, err), http.StatusInternalServerError)
// 			return
// 		}

// 		// How to redirect the user to the ticket page in frontend from here?

// 		fmt.Println("User can be redirected to ticket page with ticket ids: ", createTicketResp.TicketID)

// 		err = utils.SendMailUsingMailtrap(requestBody.Data.Customer.Email, `Your tickets are booked!`, fmt.Sprintf("Your tickets are booked! Your ticket ids are: %v", createTicketResp.TicketID))

// 		if err != nil {
// 			fmt.Println("Error sending mail: ", err)

// 			// We will not return error to payment gateway
// 			// We will log the error in redis against the idempotent key

// 			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
// 			defer cancel()

// 			var redisValueObj RedisIdempotentValue

// 			c.RedisClient.Get(ctx, requestBody.Data.Metadata.IdempotentKey).Scan(&redisValueObj)

// 			redisValueObj.ErrorMessage = fmt.Sprintf("Error sending mail: %v", err)

// 			err = c.RedisClient.Set(ctx, requestBody.Data.Metadata.IdempotentKey, redisValueObj, 5*time.Minute).Err()

// 			if err != nil {
// 				fmt.Println("Error updating idempotent key in redis: ", err)
// 			}

// 			// End of logging the error in redis

// 			// We will return 200 OK to the payment gateway
// 			// So that it does not retry the webhook
// 			// We will handle the retry logic ourselves

// 			// http.Error(w, fmt.Sprintf(`{"errors": "Error sending mail %v"}`, err), http.StatusInternalServerError)
// 			return
// 		}

// 		// Maybe I can send the ticket ids in the response of this webhook handler
// 		// Then, the frontend can redirect the user to the ticket page

// 		// But, how will the frontend know that the payment is successful and it needs to call this webhook handler?

// 		// I think the frontend can poll an endpoint to check if the payment is successful
// 		// Once the payment is successful, the frontend can call this webhook handler
// 		// The frontend can get the ticket ids from the response of this webhook handler
// 		// Then, the frontend can redirect the user to the ticket page

// 	case "payment.failed":
// 		fmt.Println("❌ Payment failed")
// 	default:
// 		fmt.Println("ℹ️ Unknown event:", requestBody.Type)
// 	}

// 	// dummyResponse.IsValid = true

// 	// jsonResponse, err := json.Marshal(dummyResponse)

// 	// if err != nil {
// 	// 	w.Header().Set("Content-Type", "application/json")
// 	// 	w.WriteHeader(http.StatusOK)
// 	// }
// 	// _, err = w.Write(jsonResponse)

// 	// if err != nil {
// 	// 	w.Header().Set("Content-Type", "application/json")
// 	// 	http.Error(w, fmt.Sprintf(`{"error": "Error writing JSON response: %v"}`, err), http.StatusInternalServerError)
// 	// 	return
// 	// }
// }

// Need to check if the payment is successful or not.
func (c *Config) CheckPaymentStatus(w http.ResponseWriter, r *http.Request) {

	var requestBody struct {
		IdempotentKey string `json:"idempotent_key"`
	}

	bodyBytes, err := io.ReadAll(r.Body)

	if err != nil {
		fmt.Println("error reading request body")
		http.Error(w, fmt.Sprintf(`{"error": "error reading request body %s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	err = json.Unmarshal(bodyBytes, &requestBody)

	if err != nil {
		fmt.Println("error unmarshalling json")
		http.Error(w, fmt.Sprintf(`{"error": "error unmarshalling reqyest json %s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	if requestBody.IdempotentKey == "" {
		http.Error(w, `{"error": "idempotentKey missing"}`, http.StatusBadRequest)
		return
	}

	var redisValue RedisIdempotentValue
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err = c.RedisClient.Get(ctx, requestBody.IdempotentKey).Scan(&redisValue)

	if err != nil {
		fmt.Println("error gettiung redis value " + err.Error())
		http.Error(w, `{"paymentSuccess": false}`, http.StatusOK)
		return
	}

	response := map[string]any{
		"paymentSuccess": redisValue.PaymentStatus == "succeeded",
		"ticketID":       redisValue.TicketID,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (c *Config) GetIdempotentKey(w http.ResponseWriter, r *http.Request) {
	idempotentKey := utils.GenerateIdempotentKey()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Store the key in Redis with a 5-minute expiration
	// Value should an object which should contain all the fields in Idempotent model from payment service

	redisValueObj := RedisIdempotentValue{
		CustomerID:      "",
		OrderIDs:        []string{},
		BookedSeatsIDs:  []int32{},
		MovieTimeSlotID: 0,
		IsTicketSent:    false,
		IsMailSent:      false,
	}

	// jsonRedisValueObj, err := json.Marshal(redisValueObj)

	// if err != nil {
	// 	w.Header().Set("Content-Type", "application/json")
	// 	http.Error(w, fmt.Sprintf(`{"error": "Error marshalling JSON response: %v"}`, err), http.StatusInternalServerError)
	// 	return
	// }

	err := c.RedisClient.Set(ctx, idempotentKey, redisValueObj, 5*time.Minute).Err()

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error storing idempotent key in Redis: %v"}`, err), http.StatusInternalServerError)
		return
	}

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
		Key string `json:"key"`
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}

	err = json.Unmarshal(bodyBytes, &requestBody)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "Error unmarshalling JSON", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Check if the key exists in Redis
	exists, err := c.RedisClient.Exists(ctx, requestBody.Key).Result()
	if err != nil || exists == 0 {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "Idempotent key is not valid or expired"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	jsonResponse, err := json.Marshal(map[string]bool{"is_valid": true})
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "Error marshalling JSON response", http.StatusInternalServerError)
		return
	}

	_, err = w.Write(jsonResponse)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "Error writing response", http.StatusInternalServerError)
		return
	}
}

func (c *Config) CommitIdempotentKey(w http.ResponseWriter, r *http.Request) {
	var requestBody struct {
		Key string `json:"key"`
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}

	err = json.Unmarshal(bodyBytes, &requestBody)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "Error unmarshalling JSON", http.StatusInternalServerError)
		return
	}

	// Commit the key to the database
	_, err = c.Payment_service.CommitIdempotentKey(context.TODO(), &payment_service.CommitIdempotentKeyRequest{
		IdempotentKey: requestBody.Key,
	})

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf("Error committing idempotent key: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	jsonResponse, err := json.Marshal(map[string]string{"message": "Idempotent key committed successfully"})
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "Error marshalling JSON response", http.StatusInternalServerError)
		return
	}

	_, err = w.Write(jsonResponse)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "Error writing response", http.StatusInternalServerError)
		return
	}
}

func (c *Config) Create_Customer(w http.ResponseWriter, r *http.Request) {
	var requestBody payment_service.CreateCustomerRequest

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "error reading request body"}`, 500)
		return
	}

	err = json.Unmarshal(bodyBytes, &requestBody)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "error unmarshalling request body : %s"}`, err.Error()), 500)
		return
	}

	if requestBody.IdempotentKey == "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "idempotent key cannot be empty", 400)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var redisResult RedisIdempotentValue

	err = c.RedisClient.Get(ctx, requestBody.IdempotentKey).Scan(&redisResult)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error getting idempotent key from Redis: %v"}`, err), http.StatusInternalServerError)
		return
	}

	response, err := c.Payment_service.CreateCustomer(context.TODO(), &requestBody)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error calling CreateCustomer rpc function: %v"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	if response.Error != "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error calling CreateCustomer rpc function: %v"}`, response.Error), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	responseBody := map[string]string{
		"message":     "Customer created successfully",
		"customer_id": response.CustomerId,
	}

	// Commit the idempotent key to the database

	// Retrieve the idempotent key details from Redis

	redisResult.CustomerID = response.CustomerId

	err = c.RedisClient.Set(ctx, requestBody.IdempotentKey, redisResult, 15*time.Minute).Err()

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error storing idempotent key in Redis: %v"}`, err), http.StatusInternalServerError)
		return
	}

	// Now commit the idempotent key to the database

	_, err = c.Payment_service.CommitIdempotentKey(context.TODO(), &payment_service.CommitIdempotentKeyRequest{
		IdempotentKey:   requestBody.IdempotentKey,
		CustomerId:      response.CustomerId,
		OrderIds:        redisResult.OrderIDs,
		MovieTimeSlotId: redisResult.MovieTimeSlotID,
		BookedSeatsIds:  redisResult.BookedSeatsIDs,
	})

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error":"Error committing idempotent key: %v"}`, err.Error()), http.StatusInternalServerError)
		return
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
		http.Error(w, fmt.Sprintf(`{"error":"error reading request body : %v"}`, err.Error()), 500)
		return
	}

	err = json.Unmarshal(bodyBytes, &requestBody)

	fmt.Println("Request body in Create order function: ", requestBody)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error":"error unmarshalling request body : %v"}`, err.Error()), 500)
		return
	}

	if requestBody.IdempotentKey == "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"idempotent key cannot be empty"}`, 400)
		return
	}

	if requestBody.MovieTimeSlotId == 0 {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"movie time slot id cannot be empty"}`, 400)
		return
	}

	if len(requestBody.SeatMatrixIDs) == 0 {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"seat matrix ids cannot be empty"}`, 400)
		return
	}

	// Check if the idempotent key exists in Redis
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	exists, err := c.RedisClient.Exists(ctx, requestBody.IdempotentKey).Result()

	if err != nil || exists == 0 {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "Idempotent key is not valid or expired"}`, http.StatusBadRequest)
		return
	}

	// Commit order ids to redis

	var redisResult RedisIdempotentValue

	err = c.RedisClient.Get(ctx, requestBody.IdempotentKey).Scan(&redisResult)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error getting idempotent key from Redis: %v"}`, err), http.StatusInternalServerError)
		return
	}

	redisResult.MovieTimeSlotID = requestBody.MovieTimeSlotId
	redisResult.BookedSeatsIDs = append(redisResult.BookedSeatsIDs, requestBody.SeatMatrixIDs...)

	// _, err = c.Payment_service.CommitIdempotentKey(context.TODO(), &payment_service.CommitIdempotentKeyRequest{
	// 	IdempotentKey: requestBody.IdempotentKey,
	// })

	// if err != nil {
	// 	w.Header().Set("Content-Type", "application/json")
	// 	http.Error(w, fmt.Sprintf(`{"error":"Error committing idempotent key: %v"}`, err.Error()), http.StatusInternalServerError)
	// 	return
	// }

	response, err := c.Payment_service.CreateOrder(context.TODO(), &requestBody)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error":"Error calling CreateOrder rpc function: %v"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	if response.Error != "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error":"Error calling CreateOrder rpc function: %v"}`, response.Error), http.StatusInternalServerError)
		return
	}

	redisResult.OrderIDs = append(redisResult.OrderIDs, response.OrderId...)

	// Commit order ids to redis, we will commit idempotent key after creating customer

	err = c.RedisClient.Set(ctx, requestBody.IdempotentKey, redisResult, 15*time.Minute).Err()

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error storing idempotent key in Redis: %v"}`, err), http.StatusInternalServerError)
		return
	}

	// Commit the idempotent key to the database
	// _, err = c.Payment_service.CommitIdempotentKey(context.TODO(), &payment_service.CommitIdempotentKeyRequest{
	// 	IdempotentKey: requestBody.IdempotentKey,
	// })

	// if err != nil {
	// 	w.Header().Set("Content-Type", "application/json")
	// 	http.Error(w, fmt.Sprintf(`{"error":"Error committing idempotent key: %v"}`, err.Error()), http.StatusInternalServerError)
	// 	return
	// }

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	_, err = w.Write(fmt.Appendf(nil, `{"order_id": "%s"}`, response.OrderId))
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "Error writing JSON response: %v"}`, err), http.StatusInternalServerError)
		return
	}
}

func (c *Config) CreatePaymentLink(w http.ResponseWriter, r *http.Request) {
	var requestBody payment_service.CreatePaymentLinkRequest

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error":"error reading request body : %s"}`, err.Error()), 500)
		return
	}

	err = json.Unmarshal(bodyBytes, &requestBody)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error":"error unmarshalling request body : %s"}`, err.Error()), 500)
		return
	}

	if requestBody.IdempotentKey == "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"idempotent key cannot be empty"}`, 400)
		return
	}

	response, err := c.Payment_service.GeneratePaymentLink(context.TODO(), &requestBody)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error":"Error calling CreatePaymentLink rpc function: %v"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	if response.Error != "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error":"Error calling CreatePaymentLink rpc function: %v"}`, response.Error), http.StatusInternalServerError)
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
