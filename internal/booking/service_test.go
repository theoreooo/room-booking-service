package booking

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"booker/internal/domain"
)

type bookingRepositoryStub struct {
	createFn           func(context.Context, *domain.Booking) (*domain.Booking, error)
	getByIDFn          func(context.Context, uuid.UUID) (*domain.Booking, error)
	cancelFn           func(context.Context, uuid.UUID) (*domain.Booking, error)
	listFn             func(context.Context, int, int) ([]*domain.BookingWithSlot, int, error)
	listByUserFutureFn func(context.Context, uuid.UUID, time.Time) ([]*domain.BookingWithSlot, error)

	createCalls int
	cancelCalls int
	lastCreate  *domain.Booking
	lastLimit   int
	lastOffset  int
	lastUserID  uuid.UUID
	lastNow     time.Time
}

func (s *bookingRepositoryStub) Create(ctx context.Context, booking *domain.Booking) (*domain.Booking, error) {
	s.createCalls++
	s.lastCreate = booking

	if s.createFn != nil {
		return s.createFn(ctx, booking)
	}

	return booking, nil
}

func (s *bookingRepositoryStub) GetByID(ctx context.Context, id uuid.UUID) (*domain.Booking, error) {
	if s.getByIDFn != nil {
		return s.getByIDFn(ctx, id)
	}

	return nil, nil
}

func (s *bookingRepositoryStub) Cancel(ctx context.Context, id uuid.UUID) (*domain.Booking, error) {
	s.cancelCalls++

	if s.cancelFn != nil {
		return s.cancelFn(ctx, id)
	}

	return nil, nil
}

func (s *bookingRepositoryStub) List(ctx context.Context, limit, offset int) ([]*domain.BookingWithSlot, int, error) {
	s.lastLimit = limit
	s.lastOffset = offset

	if s.listFn != nil {
		return s.listFn(ctx, limit, offset)
	}

	return nil, 0, nil
}

func (s *bookingRepositoryStub) ListByUserFuture(ctx context.Context, userID uuid.UUID, now time.Time) ([]*domain.BookingWithSlot, error) {
	s.lastUserID = userID
	s.lastNow = now

	if s.listByUserFutureFn != nil {
		return s.listByUserFutureFn(ctx, userID, now)
	}

	return nil, nil
}

type slotRepositoryStub struct {
	getByIDFn func(context.Context, uuid.UUID) (*domain.Slot, error)
}

func (s *slotRepositoryStub) BulkCreate(context.Context, []*domain.Slot) error {
	return nil
}

func (s *slotRepositoryStub) GetByID(ctx context.Context, id uuid.UUID) (*domain.Slot, error) {
	if s.getByIDFn != nil {
		return s.getByIDFn(ctx, id)
	}

	return nil, nil
}

func (s *slotRepositoryStub) ListAvailableByRoomAndDate(context.Context, uuid.UUID, time.Time) ([]*domain.SlotWithStatus, error) {
	return nil, nil
}

func TestServiceCreateRejectsInvalidRequest(t *testing.T) {
	service := NewService(&bookingRepositoryStub{}, &slotRepositoryStub{})

	tests := []struct {
		name    string
		booking *domain.Booking
	}{
		{name: "nil booking"},
		{
			name: "missing slot id",
			booking: &domain.Booking{
				UserID: uuid.New(),
			},
		},
		{
			name: "missing user id",
			booking: &domain.Booking{
				SlotID: uuid.New(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			created, err := service.Create(context.Background(), tt.booking)
			if err != domain.ErrInvalidRequest {
				t.Fatalf("expected ErrInvalidRequest, got %v", err)
			}
			if created != nil {
				t.Fatalf("expected nil booking, got %#v", created)
			}
		})
	}
}

func TestServiceCreateRejectsPastSlot(t *testing.T) {
	now := time.Date(2026, 3, 25, 10, 0, 0, 0, time.UTC)
	bookingRepo := &bookingRepositoryStub{}
	slotRepo := &slotRepositoryStub{
		getByIDFn: func(context.Context, uuid.UUID) (*domain.Slot, error) {
			return &domain.Slot{
				ID:      uuid.New(),
				StartAt: now.Add(-time.Minute),
			}, nil
		},
	}

	service := &Service{
		bookingRepo: bookingRepo,
		slotRepo:    slotRepo,
		now:         func() time.Time { return now },
	}

	created, err := service.Create(context.Background(), &domain.Booking{
		SlotID: uuid.New(),
		UserID: uuid.New(),
	})
	if err != domain.ErrSlotInThePast {
		t.Fatalf("expected ErrSlotInThePast, got %v", err)
	}
	if created != nil {
		t.Fatalf("expected nil booking, got %#v", created)
	}
	if bookingRepo.createCalls != 0 {
		t.Fatalf("expected booking repo Create not to be called")
	}
}

func TestServiceCreateSetsIDAndActiveStatus(t *testing.T) {
	now := time.Date(2026, 3, 25, 10, 0, 0, 0, time.UTC)
	bookingRepo := &bookingRepositoryStub{}
	slotRepo := &slotRepositoryStub{
		getByIDFn: func(context.Context, uuid.UUID) (*domain.Slot, error) {
			return &domain.Slot{
				ID:      uuid.New(),
				StartAt: now.Add(time.Minute),
			}, nil
		},
	}

	service := &Service{
		bookingRepo: bookingRepo,
		slotRepo:    slotRepo,
		now:         func() time.Time { return now },
	}

	booking := &domain.Booking{
		SlotID: uuid.New(),
		UserID: uuid.New(),
	}

	created, err := service.Create(context.Background(), booking)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if created == nil {
		t.Fatal("expected booking to be returned")
	}
	if created.ID == uuid.Nil {
		t.Fatal("expected booking ID to be generated")
	}
	if created.Status != domain.BookingStatusActive {
		t.Fatalf("expected active status, got %s", created.Status)
	}
	if bookingRepo.lastCreate != booking {
		t.Fatal("expected repo to receive the same booking pointer")
	}
}

func TestServiceListUsesDefaults(t *testing.T) {
	bookingRepo := &bookingRepositoryStub{
		listFn: func(context.Context, int, int) ([]*domain.BookingWithSlot, int, error) {
			return []*domain.BookingWithSlot{}, 42, nil
		},
	}

	service := NewService(bookingRepo, &slotRepositoryStub{})

	bookings, total, err := service.List(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if total != 42 {
		t.Fatalf("expected total 42, got %d", total)
	}
	if len(bookings) != 0 {
		t.Fatalf("expected empty bookings, got %d items", len(bookings))
	}
	if bookingRepo.lastLimit != defaultPageSize {
		t.Fatalf("expected limit %d, got %d", defaultPageSize, bookingRepo.lastLimit)
	}
	if bookingRepo.lastOffset != 0 {
		t.Fatalf("expected offset 0, got %d", bookingRepo.lastOffset)
	}
}

func TestServiceListRejectsInvalidPagination(t *testing.T) {
	service := NewService(&bookingRepositoryStub{}, &slotRepositoryStub{})

	tests := []struct {
		name     string
		page     int
		pageSize int
	}{
		{name: "page less than one", page: -1, pageSize: 20},
		{name: "page size less than one", page: 1, pageSize: -1},
		{name: "page size too large", page: 1, pageSize: 101},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bookings, total, err := service.List(context.Background(), tt.page, tt.pageSize)
			if err != domain.ErrInvalidRequest {
				t.Fatalf("expected ErrInvalidRequest, got %v", err)
			}
			if bookings != nil {
				t.Fatalf("expected nil bookings, got %#v", bookings)
			}
			if total != 0 {
				t.Fatalf("expected total 0, got %d", total)
			}
		})
	}
}

func TestServiceListMyPassesCurrentTime(t *testing.T) {
	now := time.Date(2026, 3, 25, 10, 30, 0, 0, time.UTC)
	bookingRepo := &bookingRepositoryStub{
		listByUserFutureFn: func(context.Context, uuid.UUID, time.Time) ([]*domain.BookingWithSlot, error) {
			return []*domain.BookingWithSlot{}, nil
		},
	}

	service := &Service{
		bookingRepo: bookingRepo,
		slotRepo:    &slotRepositoryStub{},
		now:         func() time.Time { return now },
	}

	userID := uuid.New()

	bookings, err := service.ListMy(context.Background(), userID)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(bookings) != 0 {
		t.Fatalf("expected empty bookings, got %d items", len(bookings))
	}
	if bookingRepo.lastUserID != userID {
		t.Fatalf("expected user ID %s, got %s", userID, bookingRepo.lastUserID)
	}
	if !bookingRepo.lastNow.Equal(now) {
		t.Fatalf("expected now %s, got %s", now, bookingRepo.lastNow)
	}
}

func TestServiceCancelRejectsForeignBooking(t *testing.T) {
	bookingRepo := &bookingRepositoryStub{
		getByIDFn: func(context.Context, uuid.UUID) (*domain.Booking, error) {
			return &domain.Booking{
				ID:     uuid.New(),
				UserID: uuid.New(),
				Status: domain.BookingStatusActive,
			}, nil
		},
	}

	service := NewService(bookingRepo, &slotRepositoryStub{})

	cancelled, err := service.Cancel(context.Background(), uuid.New(), uuid.New())
	if err != domain.ErrNotBookingOwner {
		t.Fatalf("expected ErrNotBookingOwner, got %v", err)
	}
	if cancelled != nil {
		t.Fatalf("expected nil booking, got %#v", cancelled)
	}
	if bookingRepo.cancelCalls != 0 {
		t.Fatal("expected booking repo Cancel not to be called")
	}
}

func TestServiceCancelIsIdempotent(t *testing.T) {
	existing := &domain.Booking{
		ID:     uuid.New(),
		UserID: uuid.New(),
		Status: domain.BookingStatusCancelled,
	}

	bookingRepo := &bookingRepositoryStub{
		getByIDFn: func(context.Context, uuid.UUID) (*domain.Booking, error) {
			return existing, nil
		},
	}

	service := NewService(bookingRepo, &slotRepositoryStub{})

	cancelled, err := service.Cancel(context.Background(), existing.ID, existing.UserID)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if cancelled != existing {
		t.Fatal("expected existing cancelled booking to be returned")
	}
	if bookingRepo.cancelCalls != 0 {
		t.Fatal("expected booking repo Cancel not to be called")
	}
}

func TestServiceCancelCallsRepositoryForActiveBooking(t *testing.T) {
	existing := &domain.Booking{
		ID:     uuid.New(),
		UserID: uuid.New(),
		Status: domain.BookingStatusActive,
	}
	cancelled := &domain.Booking{
		ID:     existing.ID,
		UserID: existing.UserID,
		Status: domain.BookingStatusCancelled,
	}

	bookingRepo := &bookingRepositoryStub{
		getByIDFn: func(context.Context, uuid.UUID) (*domain.Booking, error) {
			return existing, nil
		},
		cancelFn: func(context.Context, uuid.UUID) (*domain.Booking, error) {
			return cancelled, nil
		},
	}

	service := NewService(bookingRepo, &slotRepositoryStub{})

	got, err := service.Cancel(context.Background(), existing.ID, existing.UserID)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if got != cancelled {
		t.Fatal("expected cancelled booking from repository")
	}
	if bookingRepo.cancelCalls != 1 {
		t.Fatalf("expected booking repo Cancel to be called once, got %d", bookingRepo.cancelCalls)
	}
}
