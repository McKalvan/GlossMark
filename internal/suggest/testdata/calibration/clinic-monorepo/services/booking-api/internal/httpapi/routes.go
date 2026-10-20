package httpapi

import "net/http"

// @gloss implements .BookingHandler
func BookAppointment(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusCreated)
}

func Health(w http.ResponseWriter, r *http.Request) {}
