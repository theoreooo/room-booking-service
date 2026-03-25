package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"booker/internal/auth"
	"booker/internal/booking"
	"booker/internal/config"
	"booker/internal/domain"
	"booker/internal/room"
	"booker/internal/schedule"
	"booker/internal/slot"

	"github.com/google/uuid"
)

type testState struct {
	mu              sync.Mutex
	users           map[uuid.UUID]*domain.User
	rooms           map[uuid.UUID]*domain.Room
	schedulesByRoom map[uuid.UUID]*domain.Schedule
	slots           map[uuid.UUID]*domain.Slot
	bookings        map[uuid.UUID]*domain.Booking
}

func newTestState() *testState {
	return &testState{
		users:           make(map[uuid.UUID]*domain.User),
		rooms:           make(map[uuid.UUID]*domain.Room),
		schedulesByRoom: make(map[uuid.UUID]*domain.Schedule),
		slots:           make(map[uuid.UUID]*domain.Slot),
		bookings:        make(map[uuid.UUID]*domain.Booking),
	}
}

type memDummyUsers struct{ state *testState }

func (r *memDummyUsers) Ensure(_ context.Context, user *domain.User) error {
	r.state.mu.Lock()
	defer r.state.mu.Unlock()

	copyUser := *user
	if copyUser.CreatedAt.IsZero() {
		copyUser.CreatedAt = time.Now().UTC()
	}
	r.state.users[copyUser.ID] = &copyUser
	return nil
}

type memRoomRepo struct{ state *testState }

func (r *memRoomRepo) Create(_ context.Context, room *domain.Room) (*domain.Room, error) {
	r.state.mu.Lock()
	defer r.state.mu.Unlock()

	copyRoom := *room
	if copyRoom.CreatedAt.IsZero() {
		copyRoom.CreatedAt = time.Now().UTC()
	}
	r.state.rooms[copyRoom.ID] = &copyRoom
	return &copyRoom, nil
}

func (r *memRoomRepo) List(context.Context) ([]*domain.Room, error) {
	r.state.mu.Lock()
	defer r.state.mu.Unlock()

	rooms := make([]*domain.Room, 0, len(r.state.rooms))
	for _, room := range r.state.rooms {
		copyRoom := *room
		rooms = append(rooms, &copyRoom)
	}

	sort.Slice(rooms, func(i, j int) bool {
		return rooms[i].CreatedAt.After(rooms[j].CreatedAt)
	})

	return rooms, nil
}

func (r *memRoomRepo) GetByID(_ context.Context, id uuid.UUID) (*domain.Room, error) {
	r.state.mu.Lock()
	defer r.state.mu.Unlock()

	room, ok := r.state.rooms[id]
	if !ok {
		return nil, domain.ErrRoomNotFound
	}
	copyRoom := *room
	return &copyRoom, nil
}

type memScheduleRepo struct{ state *testState }

func (r *memScheduleRepo) Create(_ context.Context, schedule *domain.Schedule) (*domain.Schedule, error) {
	r.state.mu.Lock()
	defer r.state.mu.Unlock()

	if _, exists := r.state.schedulesByRoom[schedule.RoomID]; exists {
		return nil, domain.ErrScheduleExists
	}

	copySchedule := *schedule
	if copySchedule.CreatedAt.IsZero() {
		copySchedule.CreatedAt = time.Now().UTC()
	}
	r.state.schedulesByRoom[copySchedule.RoomID] = &copySchedule
	return &copySchedule, nil
}

func (r *memScheduleRepo) GetByRoomID(_ context.Context, roomID uuid.UUID) (*domain.Schedule, error) {
	r.state.mu.Lock()
	defer r.state.mu.Unlock()

	schedule, ok := r.state.schedulesByRoom[roomID]
	if !ok {
		return nil, nil
	}
	copySchedule := *schedule
	return &copySchedule, nil
}

func (r *memScheduleRepo) ListAll(context.Context) ([]*domain.Schedule, error) {
	r.state.mu.Lock()
	defer r.state.mu.Unlock()

	schedules := make([]*domain.Schedule, 0, len(r.state.schedulesByRoom))
	for _, schedule := range r.state.schedulesByRoom {
		copySchedule := *schedule
		schedules = append(schedules, &copySchedule)
	}
	return schedules, nil
}

type memSlotRepo struct{ state *testState }

func (r *memSlotRepo) BulkCreate(_ context.Context, slots []*domain.Slot) error {
	r.state.mu.Lock()
	defer r.state.mu.Unlock()

	for _, slotItem := range slots {
		conflict := false
		for _, existing := range r.state.slots {
			if existing.RoomID == slotItem.RoomID && existing.StartAt.Equal(slotItem.StartAt) {
				conflict = true
				break
			}
		}
		if conflict {
			continue
		}

		copySlot := *slotItem
		if copySlot.CreatedAt.IsZero() {
			copySlot.CreatedAt = time.Now().UTC()
		}
		r.state.slots[copySlot.ID] = &copySlot
	}

	return nil
}

func (r *memSlotRepo) GetByID(_ context.Context, id uuid.UUID) (*domain.Slot, error) {
	r.state.mu.Lock()
	defer r.state.mu.Unlock()

	slotItem, ok := r.state.slots[id]
	if !ok {
		return nil, domain.ErrSlotNotFound
	}
	copySlot := *slotItem
	return &copySlot, nil
}

func (r *memSlotRepo) ListAvailableByRoomAndDate(_ context.Context, roomID uuid.UUID, date time.Time) ([]*domain.SlotWithStatus, error) {
	r.state.mu.Lock()
	defer r.state.mu.Unlock()

	var result []*domain.SlotWithStatus
	start := date.UTC()
	end := start.Add(24 * time.Hour)

	for _, slotItem := range r.state.slots {
		if slotItem.RoomID != roomID {
			continue
		}
		if slotItem.StartAt.Before(start) || !slotItem.StartAt.Before(end) {
			continue
		}

		isBooked := false
		for _, booking := range r.state.bookings {
			if booking.SlotID == slotItem.ID && booking.Status == domain.BookingStatusActive {
				isBooked = true
				break
			}
		}

		copySlot := *slotItem
		result = append(result, &domain.SlotWithStatus{Slot: copySlot, IsBooked: isBooked})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].StartAt.Before(result[j].StartAt)
	})

	return result, nil
}

type memBookingRepo struct{ state *testState }

func (r *memBookingRepo) Create(_ context.Context, booking *domain.Booking) (*domain.Booking, error) {
	r.state.mu.Lock()
	defer r.state.mu.Unlock()

	if _, ok := r.state.slots[booking.SlotID]; !ok {
		return nil, domain.ErrSlotNotFound
	}

	for _, existing := range r.state.bookings {
		if existing.SlotID == booking.SlotID && existing.Status == domain.BookingStatusActive {
			return nil, domain.ErrSlotAlreadyBooked
		}
	}

	copyBooking := *booking
	now := time.Now().UTC()
	if copyBooking.CreatedAt.IsZero() {
		copyBooking.CreatedAt = now
	}
	copyBooking.UpdatedAt = now
	r.state.bookings[copyBooking.ID] = &copyBooking
	return &copyBooking, nil
}

func (r *memBookingRepo) GetByID(_ context.Context, id uuid.UUID) (*domain.Booking, error) {
	r.state.mu.Lock()
	defer r.state.mu.Unlock()

	booking, ok := r.state.bookings[id]
	if !ok {
		return nil, domain.ErrBookingNotFound
	}
	copyBooking := *booking
	return &copyBooking, nil
}

func (r *memBookingRepo) Cancel(_ context.Context, id uuid.UUID) (*domain.Booking, error) {
	r.state.mu.Lock()
	defer r.state.mu.Unlock()

	booking, ok := r.state.bookings[id]
	if !ok {
		return nil, domain.ErrBookingNotFound
	}
	booking.Status = domain.BookingStatusCancelled
	booking.UpdatedAt = time.Now().UTC()
	copyBooking := *booking
	return &copyBooking, nil
}

func (r *memBookingRepo) List(context.Context, int, int) ([]*domain.BookingWithSlot, int, error) {
	return nil, 0, nil
}

func (r *memBookingRepo) ListByUserFuture(_ context.Context, userID uuid.UUID, now time.Time) ([]*domain.BookingWithSlot, error) {
	r.state.mu.Lock()
	defer r.state.mu.Unlock()

	var result []*domain.BookingWithSlot
	for _, booking := range r.state.bookings {
		if booking.UserID != userID {
			continue
		}
		slotItem := r.state.slots[booking.SlotID]
		if slotItem == nil || slotItem.StartAt.Before(now) {
			continue
		}
		copyBooking := *booking
		copySlot := *slotItem
		result = append(result, &domain.BookingWithSlot{
			Booking: copyBooking,
			Slot:    copySlot,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Slot.StartAt.Before(result[j].Slot.StartAt)
	})

	return result, nil
}

func newIntegrationRouter() http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	state := newTestState()

	roomRepo := &memRoomRepo{state: state}
	scheduleRepo := &memScheduleRepo{state: state}
	slotRepo := &memSlotRepo{state: state}
	bookingRepo := &memBookingRepo{state: state}
	userStore := &memDummyUsers{state: state}

	cfg := config.JWTConfig{
		Secret:         "test-secret",
		AccessTokenTTL: time.Hour,
		AdminUserID:    "00000000-0000-0000-0000-000000000001",
		RegularUserID:  "00000000-0000-0000-0000-000000000002",
	}

	slotBuilder := slot.NewBuilder()

	return newRouter(
		cfg.Secret,
		auth.NewHTTPHandler(cfg, userStore, logger),
		room.NewHandler(room.NewService(roomRepo), logger),
		schedule.NewHandler(schedule.NewService(scheduleRepo, roomRepo, slotRepo, slotBuilder, 7, logger), logger),
		slot.NewHandler(slot.NewService(slotRepo, roomRepo), logger),
		booking.NewHandler(booking.NewService(bookingRepo, slotRepo), logger),
	)
}

func jsonRequest(t *testing.T, router http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var payload io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("failed to marshal body: %v", err)
		}
		payload = bytes.NewReader(raw)
	}

	req := httptest.NewRequest(method, path, payload)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(token) != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func dummyLoginToken(t *testing.T, router http.Handler, role string) string {
	t.Helper()

	rec := jsonRequest(t, router, http.MethodPost, "/dummyLogin", "", map[string]string{"role": role})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected dummyLogin status 200, got %d", rec.Code)
	}

	var resp struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode dummyLogin response: %v", err)
	}
	return resp.Token
}

func TestIntegrationCreateRoomScheduleAndBooking(t *testing.T) {
	router := newIntegrationRouter()
	adminToken := dummyLoginToken(t, router, "admin")
	userToken := dummyLoginToken(t, router, "user")

	roomRec := jsonRequest(t, router, http.MethodPost, "/rooms/create", adminToken, map[string]any{
		"name": "Board",
	})
	if roomRec.Code != http.StatusCreated {
		t.Fatalf("expected room create status 201, got %d", roomRec.Code)
	}

	var roomResp struct {
		Room struct {
			ID uuid.UUID `json:"id"`
		} `json:"room"`
	}
	if err := json.NewDecoder(roomRec.Body).Decode(&roomResp); err != nil {
		t.Fatalf("failed to decode room response: %v", err)
	}

	tomorrow := time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")
	scheduleRec := jsonRequest(t, router, http.MethodPost, "/rooms/"+roomResp.Room.ID.String()+"/schedule/create", adminToken, map[string]any{
		"daysOfWeek": []int{1, 2, 3, 4, 5, 6, 7},
		"startTime":  "10:00",
		"endTime":    "11:00",
	})
	if scheduleRec.Code != http.StatusCreated {
		t.Fatalf("expected schedule create status 201, got %d", scheduleRec.Code)
	}

	slotsRec := jsonRequest(t, router, http.MethodGet, "/rooms/"+roomResp.Room.ID.String()+"/slots/list?date="+tomorrow, userToken, nil)
	if slotsRec.Code != http.StatusOK {
		t.Fatalf("expected slots list status 200, got %d", slotsRec.Code)
	}

	var slotsResp struct {
		Slots []struct {
			ID uuid.UUID `json:"id"`
		} `json:"slots"`
	}
	if err := json.NewDecoder(slotsRec.Body).Decode(&slotsResp); err != nil {
		t.Fatalf("failed to decode slots response: %v", err)
	}
	if len(slotsResp.Slots) == 0 {
		t.Fatal("expected at least one available slot")
	}

	bookingRec := jsonRequest(t, router, http.MethodPost, "/bookings/create", userToken, map[string]any{
		"slotId": slotsResp.Slots[0].ID,
	})
	if bookingRec.Code != http.StatusCreated {
		t.Fatalf("expected booking create status 201, got %d", bookingRec.Code)
	}

	var bookingResp struct {
		Booking struct {
			Status string    `json:"status"`
			UserID uuid.UUID `json:"userId"`
		} `json:"booking"`
	}
	if err := json.NewDecoder(bookingRec.Body).Decode(&bookingResp); err != nil {
		t.Fatalf("failed to decode booking response: %v", err)
	}
	if bookingResp.Booking.Status != string(domain.BookingStatusActive) {
		t.Fatalf("expected active booking, got %s", bookingResp.Booking.Status)
	}
	if bookingResp.Booking.UserID != uuid.MustParse("00000000-0000-0000-0000-000000000002") {
		t.Fatalf("expected user id from dummyLogin token, got %s", bookingResp.Booking.UserID)
	}
}

func TestIntegrationUserCanCancelBookingIdempotently(t *testing.T) {
	router := newIntegrationRouter()
	adminToken := dummyLoginToken(t, router, "admin")
	userToken := dummyLoginToken(t, router, "user")

	roomRec := jsonRequest(t, router, http.MethodPost, "/rooms/create", adminToken, map[string]any{
		"name": "Focus",
	})

	var roomResp struct {
		Room struct {
			ID uuid.UUID `json:"id"`
		} `json:"room"`
	}
	if err := json.NewDecoder(roomRec.Body).Decode(&roomResp); err != nil {
		t.Fatalf("failed to decode room response: %v", err)
	}

	jsonRequest(t, router, http.MethodPost, "/rooms/"+roomResp.Room.ID.String()+"/schedule/create", adminToken, map[string]any{
		"daysOfWeek": []int{1, 2, 3, 4, 5, 6, 7},
		"startTime":  "10:00",
		"endTime":    "11:00",
	})

	tomorrow := time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")
	slotsRec := jsonRequest(t, router, http.MethodGet, "/rooms/"+roomResp.Room.ID.String()+"/slots/list?date="+tomorrow, userToken, nil)

	var slotsResp struct {
		Slots []struct {
			ID uuid.UUID `json:"id"`
		} `json:"slots"`
	}
	if err := json.NewDecoder(slotsRec.Body).Decode(&slotsResp); err != nil {
		t.Fatalf("failed to decode slots response: %v", err)
	}

	bookingRec := jsonRequest(t, router, http.MethodPost, "/bookings/create", userToken, map[string]any{
		"slotId": slotsResp.Slots[0].ID,
	})

	var bookingResp struct {
		Booking struct {
			ID uuid.UUID `json:"id"`
		} `json:"booking"`
	}
	if err := json.NewDecoder(bookingRec.Body).Decode(&bookingResp); err != nil {
		t.Fatalf("failed to decode booking response: %v", err)
	}

	cancelPath := "/bookings/" + bookingResp.Booking.ID.String() + "/cancel"
	cancelRec := jsonRequest(t, router, http.MethodPost, cancelPath, userToken, nil)
	if cancelRec.Code != http.StatusOK {
		t.Fatalf("expected cancel status 200, got %d", cancelRec.Code)
	}

	secondCancelRec := jsonRequest(t, router, http.MethodPost, cancelPath, userToken, nil)
	if secondCancelRec.Code != http.StatusOK {
		t.Fatalf("expected idempotent cancel status 200, got %d", secondCancelRec.Code)
	}

	var cancelResp struct {
		Booking struct {
			Status string `json:"status"`
		} `json:"booking"`
	}
	if err := json.NewDecoder(secondCancelRec.Body).Decode(&cancelResp); err != nil {
		t.Fatalf("failed to decode cancel response: %v", err)
	}
	if cancelResp.Booking.Status != string(domain.BookingStatusCancelled) {
		t.Fatalf("expected cancelled status, got %s", cancelResp.Booking.Status)
	}
}
