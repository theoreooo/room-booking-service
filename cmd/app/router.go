package main

import (
	"net/http"
	"time"

	"booker/internal/auth"
	"booker/internal/booking"
	"booker/internal/middleware"
	"booker/internal/room"
	"booker/internal/schedule"
	"booker/internal/slot"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

func newRouter(
	jwtSecret string,
	authHandler *auth.Handler,
	roomHandler *room.Handler,
	scheduleHandler *schedule.Handler,
	slotHandler *slot.Handler,
	bookingHandler *booking.Handler,
) http.Handler {
	r := chi.NewRouter()
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Timeout(5 * time.Second))

	r.Get("/_info", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r.Post("/dummyLogin", authHandler.DummyLogin)

	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth(jwtSecret))
		roomHandler.Register(r)
		scheduleHandler.Register(r)
		slotHandler.Register(r)
		bookingHandler.Register(r)
	})

	return r
}
