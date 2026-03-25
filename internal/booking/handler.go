package booking

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
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
	r.Route("/bookings", func(r chi.Router) {
		r.Post("/create", h.Create)
		r.Get("/list", h.List)
		r.Get("/my", h.ListMy)
		r.Post("/{bookingID}/cancel", h.Cancel)
	})
}

type createBookingRequest struct {
	SlotID               string `json:"slotId"`
	CreateConferenceLink bool   `json:"createConferenceLink"`
}

type bookingResponse struct {
	ID             uuid.UUID `json:"id"`
	SlotID         uuid.UUID `json:"slotId"`
	UserID         uuid.UUID `json:"userId"`
	Status         string    `json:"status" enums:"active,cancelled"`
	ConferenceLink *string   `json:"conferenceLink,omitempty"`
	CreatedAt      string    `json:"createdAt"`
}

type paginationResponse struct {
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
	Total    int `json:"total"`
}

type bookingEnvelope struct {
	Booking bookingResponse `json:"booking"`
}

type bookingsEnvelope struct {
	Bookings   []bookingResponse  `json:"bookings"`
	Pagination paginationResponse `json:"pagination,omitempty"`
}

func toBookingResponse(b *domain.Booking) bookingResponse {
	return bookingResponse{
		ID:             b.ID,
		SlotID:         b.SlotID,
		UserID:         b.UserID,
		Status:         string(b.Status),
		ConferenceLink: b.ConferenceLink,
		CreatedAt:      b.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func toBookingResponseFromWithSlot(b *domain.BookingWithSlot) bookingResponse {
	return toBookingResponse(&b.Booking)
}

// Create godoc
// @Summary Create a booking
// @Tags Bookings
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param request body createBookingRequest true "Create booking request"
// @Success 201 {object} bookingEnvelope
// @Failure 400 {object} httputil.ErrorResponse
// @Failure 401 {object} httputil.ErrorResponse
// @Failure 403 {object} httputil.ErrorResponse
// @Failure 404 {object} httputil.ErrorResponse
// @Failure 409 {object} httputil.ErrorResponse
// @Failure 503 {object} httputil.ErrorResponse
// @Failure 500 {object} httputil.ErrorResponse
// @Router /bookings/create [post]
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	claims, ok := middleware.ClaimsFromCtx(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, domain.ErrUnauthorized)
		return
	}

	if claims.Role != domain.UserRoleUser {
		httputil.WriteError(w, http.StatusForbidden, domain.ErrForbidden)
		return
	}

	var req createBookingRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, domain.ErrInvalidRequest)
		return
	}

	slotID, err := uuid.Parse(req.SlotID)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, domain.ErrInvalidRequest)
		return
	}

	booking := &domain.Booking{
		SlotID: slotID,
		UserID: claims.UserID,
	}

	created, err := h.service.Create(r.Context(), booking, CreateOptions{
		CreateConferenceLink: req.CreateConferenceLink,
	})
	if err != nil {
		httputil.HandleError(w, h.log, err)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, map[string]any{
		"booking": toBookingResponse(created),
	})
}

// List godoc
// @Summary List all bookings
// @Tags Bookings
// @Security BearerAuth
// @Produce json
// @Param page query int false "Page number"
// @Param pageSize query int false "Page size"
// @Success 200 {object} bookingsEnvelope
// @Failure 400 {object} httputil.ErrorResponse
// @Failure 401 {object} httputil.ErrorResponse
// @Failure 403 {object} httputil.ErrorResponse
// @Failure 500 {object} httputil.ErrorResponse
// @Router /bookings/list [get]
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
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

	page, err := parseQueryInt(r.URL.Query().Get("page"))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, domain.ErrInvalidRequest)
		return
	}

	pageSize, err := parseQueryInt(r.URL.Query().Get("pageSize"))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, domain.ErrInvalidRequest)
		return
	}

	page, pageSize, _, err = normalizePagination(page, pageSize)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, domain.ErrInvalidRequest)
		return
	}

	bookings, total, err := h.service.List(r.Context(), page, pageSize)
	if err != nil {
		httputil.HandleError(w, h.log, err)
		return
	}

	resp := make([]bookingResponse, len(bookings))
	for i, booking := range bookings {
		resp[i] = toBookingResponseFromWithSlot(booking)
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"bookings": resp,
		"pagination": paginationResponse{
			Page:     page,
			PageSize: pageSize,
			Total:    total,
		},
	})
}

// ListMy godoc
// @Summary List current user's future bookings
// @Tags Bookings
// @Security BearerAuth
// @Produce json
// @Success 200 {object} bookingsEnvelope
// @Failure 401 {object} httputil.ErrorResponse
// @Failure 403 {object} httputil.ErrorResponse
// @Failure 500 {object} httputil.ErrorResponse
// @Router /bookings/my [get]
func (h *Handler) ListMy(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	claims, ok := middleware.ClaimsFromCtx(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, domain.ErrUnauthorized)
		return
	}

	if claims.Role != domain.UserRoleUser {
		httputil.WriteError(w, http.StatusForbidden, domain.ErrForbidden)
		return
	}

	bookings, err := h.service.ListMy(r.Context(), claims.UserID)
	if err != nil {
		httputil.HandleError(w, h.log, err)
		return
	}

	resp := make([]bookingResponse, len(bookings))
	for i, booking := range bookings {
		resp[i] = toBookingResponseFromWithSlot(booking)
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"bookings": resp,
	})
}

// Cancel godoc
// @Summary Cancel a booking
// @Tags Bookings
// @Security BearerAuth
// @Produce json
// @Param bookingID path string true "Booking ID" format(uuid)
// @Success 200 {object} bookingEnvelope
// @Failure 400 {object} httputil.ErrorResponse
// @Failure 401 {object} httputil.ErrorResponse
// @Failure 403 {object} httputil.ErrorResponse
// @Failure 404 {object} httputil.ErrorResponse
// @Failure 500 {object} httputil.ErrorResponse
// @Router /bookings/{bookingID}/cancel [post]
func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	claims, ok := middleware.ClaimsFromCtx(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, domain.ErrUnauthorized)
		return
	}

	if claims.Role != domain.UserRoleUser {
		httputil.WriteError(w, http.StatusForbidden, domain.ErrForbidden)
		return
	}

	bookingIDStr := chi.URLParam(r, "bookingID")
	bookingID, err := uuid.Parse(bookingIDStr)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, domain.ErrInvalidRequest)
		return
	}

	booking, err := h.service.Cancel(r.Context(), bookingID, claims.UserID)
	if err != nil {
		httputil.HandleError(w, h.log, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"booking": toBookingResponse(booking),
	})
}

func parseQueryInt(raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}

	return value, nil
}
