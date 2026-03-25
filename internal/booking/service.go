package booking

import (
	"context"
	"time"

	"github.com/google/uuid"

	"booker/internal/domain"
)

const (
	defaultPage     = 1
	defaultPageSize = 20
	maxPageSize     = 100
)

type Service struct {
	bookingRepo domain.BookingRepository
	slotRepo    domain.SlotRepository
	now         func() time.Time
}

func NewService(bookingRepo domain.BookingRepository, slotRepo domain.SlotRepository) *Service {
	return &Service{
		bookingRepo: bookingRepo,
		slotRepo:    slotRepo,
		now:         time.Now,
	}
}

func (s *Service) Create(ctx context.Context, booking *domain.Booking) (*domain.Booking, error) {
	if booking == nil || booking.SlotID == uuid.Nil || booking.UserID == uuid.Nil {
		return nil, domain.ErrInvalidRequest
	}

	slot, err := s.slotRepo.GetByID(ctx, booking.SlotID)
	if err != nil {
		return nil, err
	}

	if slot.StartAt.Before(s.now().UTC()) {
		return nil, domain.ErrSlotInThePast
	}

	booking.ID = uuid.New()
	booking.Status = domain.BookingStatusActive

	return s.bookingRepo.Create(ctx, booking)
}

func (s *Service) List(ctx context.Context, page, pageSize int) ([]*domain.BookingWithSlot, int, error) {
	page, pageSize, offset, err := normalizePagination(page, pageSize)
	if err != nil {
		return nil, 0, err
	}

	bookings, total, err := s.bookingRepo.List(ctx, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}

	return bookings, total, nil
}

func (s *Service) ListMy(ctx context.Context, userID uuid.UUID) ([]*domain.BookingWithSlot, error) {
	if userID == uuid.Nil {
		return nil, domain.ErrInvalidRequest
	}

	return s.bookingRepo.ListByUserFuture(ctx, userID, s.now().UTC())
}

func (s *Service) Cancel(ctx context.Context, bookingID, userID uuid.UUID) (*domain.Booking, error) {
	if bookingID == uuid.Nil || userID == uuid.Nil {
		return nil, domain.ErrInvalidRequest
	}

	booking, err := s.bookingRepo.GetByID(ctx, bookingID)
	if err != nil {
		return nil, err
	}

	if booking.UserID != userID {
		return nil, domain.ErrNotBookingOwner
	}

	if booking.Status == domain.BookingStatusCancelled {
		return booking, nil
	}

	return s.bookingRepo.Cancel(ctx, bookingID)
}

func normalizePagination(page, pageSize int) (int, int, int, error) {
	if page == 0 {
		page = defaultPage
	}

	if pageSize == 0 {
		pageSize = defaultPageSize
	}

	if page < 1 || pageSize < 1 || pageSize > maxPageSize {
		return 0, 0, 0, domain.ErrInvalidRequest
	}

	offset := (page - 1) * pageSize

	return page, pageSize, offset, nil
}
