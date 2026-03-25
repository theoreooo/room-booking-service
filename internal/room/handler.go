package room

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
	r.Route("/rooms", func(r chi.Router) {
		r.Get("/list", h.List)
		r.Post("/create", h.Create)
	})
}

type createRoomRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Capacity    *int    `json:"capacity"`
}

type roomResponse struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	Capacity    *int      `json:"capacity,omitempty"`
	CreatedAt   string    `json:"createdAt,omitempty"`
}

type roomEnvelope struct {
	Room roomResponse `json:"room"`
}

type roomsEnvelope struct {
	Rooms []roomResponse `json:"rooms"`
}

func toRoomResponse(r *domain.Room) roomResponse {
	return roomResponse{
		ID:          r.ID,
		Name:        r.Name,
		Description: r.Description,
		Capacity:    r.Capacity,
		CreatedAt:   r.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// Create godoc
// @Summary Create a room
// @Tags Rooms
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param request body createRoomRequest true "Create room request"
// @Success 201 {object} roomEnvelope
// @Failure 400 {object} httputil.ErrorResponse
// @Failure 401 {object} httputil.ErrorResponse
// @Failure 403 {object} httputil.ErrorResponse
// @Failure 500 {object} httputil.ErrorResponse
// @Router /rooms/create [post]
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

	var req createRoomRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, domain.ErrInvalidRequest)
		return
	}

	room := &domain.Room{
		Name:        req.Name,
		Description: req.Description,
		Capacity:    req.Capacity,
	}

	created, err := h.service.Create(r.Context(), room)
	if err != nil {
		httputil.HandleError(w, h.log, err)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, map[string]any{
		"room": toRoomResponse(created),
	})
}

// List godoc
// @Summary List rooms
// @Tags Rooms
// @Security BearerAuth
// @Produce json
// @Success 200 {object} roomsEnvelope
// @Failure 401 {object} httputil.ErrorResponse
// @Failure 500 {object} httputil.ErrorResponse
// @Router /rooms/list [get]
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	rooms, err := h.service.List(r.Context())
	if err != nil {
		httputil.HandleError(w, h.log, err)
		return
	}

	resp := make([]roomResponse, len(rooms))
	for i, room := range rooms {
		resp[i] = toRoomResponse(room)
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"rooms": resp,
	})
}
