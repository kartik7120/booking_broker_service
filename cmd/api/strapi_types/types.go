package strapitypes

type StrapiSeatType struct {
	SeatNumber string `json:"seat_number"`
	Row        int    `json:"row"`
	Column     int    `json:"column"`
	Price      int    `json:"price"`
	Type       string `json:"type"`
	VenueID    int    `json:"venue_id"`
	SeatID     int    `json:"seat_id"`
}

type StrapiMovieTimeSlotType struct {
	StartTime   string `json:"start_time"`
	EndTime     string `json:"end_time"`
	Duration    int    `json:"duration"`
	MovieFormat string `json:"movie_format"`
	MovieID     int    `json:"movie_id"`
	VenueID     int    `json:"venue_id"`
	TimeSlotID  int    `json:"time_slot_id"`
	Date        string `json:"date"`
}

type StrapiCastType struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	CharacterName string `json:"character_name"`
	PhotoURL      string `json:"photo_url"`
	MovieID       int    `json:"movie_id"`
}

type StrapiWebHookType struct {
	Event     string `json:"event"`
	CreatedAt string `json:"createdAt"`
	Model     string `json:"model"`
	Entry     any    `json:"entry"`
}
