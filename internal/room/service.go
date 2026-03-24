package room

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"booker/internal/domain"
)

type Service struct {
	repo domain.RoomRepository
}

func NewService(repo domain.RoomRepository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, room *domain.Room) (*domain.Room, error) {
	if room == nil || strings.TrimSpace(room.Name) == "" {
		return nil, domain.ErrInvalidRequest
	}

	if room.Capacity != nil && *room.Capacity <= 0 {
		return nil, domain.ErrInvalidRequest
	}

	room.ID = uuid.New()

	return s.repo.Create(ctx, room)
}

func (s *Service) List(ctx context.Context) ([]*domain.Room, error) {
	return s.repo.List(ctx)
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (*domain.Room, error) {
	if id == uuid.Nil {
		return nil, domain.ErrInvalidRequest
	}

	return s.repo.GetByID(ctx, id)
}
