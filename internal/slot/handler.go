package slot

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"booker/internal/domain"
	"booker/internal/httputil"
)

type Handler struct {
	service *Service
	log     *slog.Logger
}

func NewHandler(service *Service, log *slog.Logger) *Handler {
	return &Handler{service: service, log: log}
}

func (h *Handler) Register(r chi.Router) {
	r.Route("/rooms/{roomID}/slots", func(r chi.Router) {
		r.Get("/list", h.ListAvailable)
	})
}

type listSlotsRequest struct {
	Date string `json:"date"`
}

type slotResponse struct {
	ID        uuid.UUID `json:"id"`
	RoomID    uuid.UUID `json:"roomId"`
	StartAt   string    `json:"startAt"`
	EndAt     string    `json:"endAt"`
	IsBooked  bool      `json:"isBooked"`
	CreatedAt string    `json:"createdAt"`
}

func toSlotResponse(s *domain.Slot) slotResponse {
	return slotResponse{
		ID:        s.ID,
		RoomID:    s.RoomID,
		StartAt:   s.StartAt.UTC().Format(time.RFC3339),
		EndAt:     s.EndAt.UTC().Format(time.RFC3339),
		IsBooked:  false,
		CreatedAt: s.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func (h *Handler) ListAvailable(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	roomIDStr := chi.URLParam(r, "roomID")
	roomID, err := uuid.Parse(roomIDStr)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, domain.ErrInvalidRequest)
		return
	}

	dateStr := r.URL.Query().Get("date")
	if dateStr == "" {
		httputil.WriteError(w, http.StatusBadRequest, domain.ErrInvalidRequest)
		return
	}

	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, domain.ErrInvalidRequest)
		return
	}

	slots, err := h.service.ListAvailable(r.Context(), roomID, date)
	if err != nil {
		httputil.HandleError(w, h.log, err)
		return
	}

	resp := make([]slotResponse, len(slots))
	for i, s := range slots {
		resp[i] = toSlotResponse(s)
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"slots": resp,
	})
}
