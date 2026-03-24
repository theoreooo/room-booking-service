package slot

import (
	"context"
	"time"

	"github.com/google/uuid"

	"booker/internal/domain"
)

type Service struct {
	slotRepo domain.SlotRepository
	roomRepo domain.RoomRepository
}

func NewService(slotRepo domain.SlotRepository, roomRepo domain.RoomRepository) *Service {
	return &Service{
		slotRepo: slotRepo,
		roomRepo: roomRepo,
	}
}

func (s *Service) ListAvailable(ctx context.Context, roomID uuid.UUID, date time.Time) ([]*domain.Slot, error) {

	if roomID == uuid.Nil || date.IsZero() {
		return nil, domain.ErrInvalidRequest
	}

	_, err := s.roomRepo.GetByID(ctx, roomID)
	if err != nil {
		return nil, err
	}

	date = date.UTC().Truncate(24 * time.Hour)

	slotsWithStatus, err := s.slotRepo.ListAvailableByRoomAndDate(ctx, roomID, date)
	if err != nil {
		return nil, err
	}

	slots := make([]*domain.Slot, 0, len(slotsWithStatus))

	for _, s := range slotsWithStatus {
		if s.IsBooked {
			continue
		}

		slots = append(slots, &domain.Slot{
			ID:        s.ID,
			RoomID:    s.RoomID,
			StartAt:   s.StartAt,
			EndAt:     s.EndAt,
			CreatedAt: s.CreatedAt,
		})
	}

	return slots, nil
}
