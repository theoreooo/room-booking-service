package booking

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"booker/internal/conference"
	"booker/internal/domain"
	"booker/internal/middleware"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func bookingToken(t *testing.T, secret string, userID uuid.UUID, role domain.UserRole) string {
	t.Helper()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": userID.String(),
		"role":    string(role),
		"exp":     time.Now().Add(time.Hour).Unix(),
	})

	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	return signed
}

func newBookingTestRouter(secret string, service *Service) http.Handler {
	handler := NewHandler(service, slog.New(slog.NewTextHandler(io.Discard, nil)))
	r := chi.NewRouter()
	r.Use(middleware.Auth(secret))
	handler.Register(r)
	return r
}

func TestHandlerCreateUsesUserIDFromToken(t *testing.T) {
	secret := "secret"
	slotID := uuid.New()
	userID := uuid.New()
	now := time.Date(2026, 3, 25, 10, 0, 0, 0, time.UTC)

	bookingRepo := &bookingRepositoryStub{
		createFn: func(_ context.Context, booking *domain.Booking) (*domain.Booking, error) {
			booking.CreatedAt = now
			return booking, nil
		},
	}
	slotRepo := &slotRepositoryStub{
		getByIDFn: func(context.Context, uuid.UUID) (*domain.Slot, error) {
			return &domain.Slot{
				ID:      slotID,
				StartAt: now.Add(time.Hour),
			}, nil
		},
	}

	service := &Service{
		bookingRepo: bookingRepo,
		slotRepo:    slotRepo,
		now:         func() time.Time { return now },
	}

	req := httptest.NewRequest(http.MethodPost, "/bookings/create", strings.NewReader(`{"slotId":"`+slotID.String()+`"}`))
	req.Header.Set("Authorization", "Bearer "+bookingToken(t, secret, userID, domain.UserRoleUser))
	rec := httptest.NewRecorder()

	newBookingTestRouter(secret, service).ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", rec.Code)
	}
	if bookingRepo.lastCreate == nil {
		t.Fatal("expected repo to receive booking")
	}
	if bookingRepo.lastCreate.UserID != userID {
		t.Fatalf("expected user id from token %s, got %s", userID, bookingRepo.lastCreate.UserID)
	}
}

func TestHandlerCreateRejectsAdminRole(t *testing.T) {
	secret := "secret"
	service := NewService(&bookingRepositoryStub{}, &slotRepositoryStub{}, nil)

	req := httptest.NewRequest(http.MethodPost, "/bookings/create", strings.NewReader(`{"slotId":"`+uuid.NewString()+`"}`))
	req.Header.Set("Authorization", "Bearer "+bookingToken(t, secret, uuid.New(), domain.UserRoleAdmin))
	rec := httptest.NewRecorder()

	newBookingTestRouter(secret, service).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", rec.Code)
	}
}

func TestHandlerListReturnsDefaults(t *testing.T) {
	secret := "secret"
	now := time.Date(2026, 3, 25, 10, 0, 0, 0, time.UTC)
	bookingRepo := &bookingRepositoryStub{
		listFn: func(context.Context, int, int) ([]*domain.BookingWithSlot, int, error) {
			return []*domain.BookingWithSlot{{
				Booking: domain.Booking{
					ID:        uuid.New(),
					SlotID:    uuid.New(),
					UserID:    uuid.New(),
					Status:    domain.BookingStatusActive,
					CreatedAt: now,
				},
			}}, 1, nil
		},
	}
	service := NewService(bookingRepo, &slotRepositoryStub{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/bookings/list", nil)
	req.Header.Set("Authorization", "Bearer "+bookingToken(t, secret, uuid.New(), domain.UserRoleAdmin))
	rec := httptest.NewRecorder()

	newBookingTestRouter(secret, service).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp struct {
		Pagination struct {
			Page     int `json:"page"`
			PageSize int `json:"pageSize"`
		} `json:"pagination"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Pagination.Page != defaultPage {
		t.Fatalf("expected default page %d, got %d", defaultPage, resp.Pagination.Page)
	}
	if resp.Pagination.PageSize != defaultPageSize {
		t.Fatalf("expected default page size %d, got %d", defaultPageSize, resp.Pagination.PageSize)
	}
}

func TestHandlerListRejectsInvalidPagination(t *testing.T) {
	secret := "secret"
	service := NewService(&bookingRepositoryStub{}, &slotRepositoryStub{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/bookings/list?page=abc", nil)
	req.Header.Set("Authorization", "Bearer "+bookingToken(t, secret, uuid.New(), domain.UserRoleAdmin))
	rec := httptest.NewRecorder()

	newBookingTestRouter(secret, service).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestHandlerListMyReturnsBookings(t *testing.T) {
	secret := "secret"
	userID := uuid.New()
	now := time.Date(2026, 3, 25, 10, 0, 0, 0, time.UTC)

	bookingRepo := &bookingRepositoryStub{
		listByUserFutureFn: func(context.Context, uuid.UUID, time.Time) ([]*domain.BookingWithSlot, error) {
			return []*domain.BookingWithSlot{{
				Booking: domain.Booking{
					ID:        uuid.New(),
					SlotID:    uuid.New(),
					UserID:    userID,
					Status:    domain.BookingStatusActive,
					CreatedAt: now,
				},
			}}, nil
		},
	}
	service := &Service{
		bookingRepo: bookingRepo,
		slotRepo:    &slotRepositoryStub{},
		now:         func() time.Time { return now },
	}

	req := httptest.NewRequest(http.MethodGet, "/bookings/my", nil)
	req.Header.Set("Authorization", "Bearer "+bookingToken(t, secret, userID, domain.UserRoleUser))
	rec := httptest.NewRecorder()

	newBookingTestRouter(secret, service).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

func TestHandlerCreateReturnsConferenceLinkWhenRequested(t *testing.T) {
	secret := "secret"
	slotID := uuid.New()
	userID := uuid.New()
	now := time.Date(2026, 3, 25, 10, 0, 0, 0, time.UTC)

	bookingRepo := &bookingRepositoryStub{
		createFn: func(_ context.Context, booking *domain.Booking) (*domain.Booking, error) {
			booking.CreatedAt = now
			return booking, nil
		},
	}
	slotRepo := &slotRepositoryStub{
		getByIDFn: func(context.Context, uuid.UUID) (*domain.Slot, error) {
			return &domain.Slot{
				ID:      slotID,
				StartAt: now.Add(time.Hour),
				EndAt:   now.Add(90 * time.Minute),
			}, nil
		},
	}

	service := &Service{
		bookingRepo: bookingRepo,
		slotRepo:    slotRepo,
		conference: &conferenceServiceStub{
			createLinkFn: func(context.Context, conference.Request) (string, error) {
				return "https://conference.mock.local/rooms/test-link", nil
			},
		},
		now: func() time.Time { return now },
	}

	req := httptest.NewRequest(http.MethodPost, "/bookings/create", strings.NewReader(`{"slotId":"`+slotID.String()+`","createConferenceLink":true}`))
	req.Header.Set("Authorization", "Bearer "+bookingToken(t, secret, userID, domain.UserRoleUser))
	rec := httptest.NewRecorder()

	newBookingTestRouter(secret, service).ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", rec.Code)
	}

	var resp struct {
		Booking struct {
			ConferenceLink string `json:"conferenceLink"`
		} `json:"booking"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Booking.ConferenceLink != "https://conference.mock.local/rooms/test-link" {
		t.Fatalf("expected conference link in response, got %q", resp.Booking.ConferenceLink)
	}
}

func TestHandlerCancelReturnsCancelledBooking(t *testing.T) {
	secret := "secret"
	userID := uuid.New()
	bookingID := uuid.New()

	bookingRepo := &bookingRepositoryStub{
		getByIDFn: func(context.Context, uuid.UUID) (*domain.Booking, error) {
			return &domain.Booking{
				ID:     bookingID,
				UserID: userID,
				Status: domain.BookingStatusActive,
			}, nil
		},
		cancelFn: func(context.Context, uuid.UUID) (*domain.Booking, error) {
			return &domain.Booking{
				ID:     bookingID,
				UserID: userID,
				Status: domain.BookingStatusCancelled,
			}, nil
		},
	}
	service := NewService(bookingRepo, &slotRepositoryStub{}, nil)

	req := httptest.NewRequest(http.MethodPost, "/bookings/"+bookingID.String()+"/cancel", nil)
	req = withBookingRouteParam(req, "bookingID", bookingID.String())
	req.Header.Set("Authorization", "Bearer "+bookingToken(t, secret, userID, domain.UserRoleUser))
	rec := httptest.NewRecorder()

	newBookingTestRouter(secret, service).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

func withBookingRouteParam(r *http.Request, key, value string) *http.Request {
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, routeCtx))
}
