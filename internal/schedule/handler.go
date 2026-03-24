package schedule

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"booker/internal/domain"
	"booker/internal/httputil"
	"booker/internal/middleware"
)

type Handler struct {
	service *Service
	log     *slog.Logger
}

func NewHandler(service *Service, log *slog.Logger) *Handler {
	return &Handler{service: service, log: log}
}

func (h *Handler) Register(r chi.Router) {
	r.Route("/rooms/{roomID}/schedule", func(r chi.Router) {
		r.Post("/create", h.Create)
	})
}

type createScheduleRequest struct {
	DaysOfWeek []int16 `json:"daysOfWeek"`
	StartTime  string  `json:"startTime"`
	EndTime    string  `json:"endTime"`
}

type scheduleResponse struct {
	ID         uuid.UUID `json:"id"`
	RoomID     uuid.UUID `json:"roomId"`
	DaysOfWeek []int16   `json:"daysOfWeek"`
	StartTime  string    `json:"startTime"`
	EndTime    string    `json:"endTime"`
	CreatedAt  string    `json:"createdAt"`
}

func toScheduleResponse(s *domain.Schedule) scheduleResponse {
	return scheduleResponse{
		ID:         s.ID,
		RoomID:     s.RoomID,
		DaysOfWeek: s.DaysOfWeek,
		StartTime:  s.StartTime.Format("15:04"),
		EndTime:    s.EndTime.Format("15:04"),
		CreatedAt:  s.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	claims, ok := middleware.ClaimsFromCtx(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, domain.ErrUnauthorized)
		return
	}

	if claims.Role != domain.UserRoleAdmin {
		httputil.WriteError(w, http.StatusForbidden, domain.ErrForbidden)
		return
	}

	roomIDStr := chi.URLParam(r, "roomID")
	roomID, err := uuid.Parse(roomIDStr)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, domain.ErrInvalidRequest)
		return
	}

	var req createScheduleRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, domain.ErrInvalidRequest)
		return
	}

	startTime, err := time.Parse("15:04", req.StartTime)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, domain.ErrInvalidRequest)
		return
	}

	endTime, err := time.Parse("15:04", req.EndTime)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, domain.ErrInvalidRequest)
		return
	}

	schedule := &domain.Schedule{
		RoomID:     roomID,
		DaysOfWeek: req.DaysOfWeek,
		StartTime:  startTime,
		EndTime:    endTime,
	}

	created, err := h.service.Create(r.Context(), schedule)
	if err != nil {
		httputil.HandleError(w, h.log, err)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, map[string]any{
		"schedule": toScheduleResponse(created),
	})
}
