package worker

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"booker/internal/domain"

	"github.com/google/uuid"
)

type workerScheduleRepoStub struct {
	listAllFn func(context.Context) ([]*domain.Schedule, error)
}

func (s *workerScheduleRepoStub) Create(context.Context, *domain.Schedule) (*domain.Schedule, error) {
	return nil, nil
}
func (s *workerScheduleRepoStub) GetByRoomID(context.Context, uuid.UUID) (*domain.Schedule, error) {
	return nil, nil
}
func (s *workerScheduleRepoStub) ListAll(ctx context.Context) ([]*domain.Schedule, error) {
	if s.listAllFn != nil {
		return s.listAllFn(ctx)
	}
	return nil, nil
}

type workerSlotRepoStub struct {
	bulkCreateCalls int
}

func (s *workerSlotRepoStub) BulkCreate(context.Context, []*domain.Slot) error {
	s.bulkCreateCalls++
	return nil
}
func (s *workerSlotRepoStub) GetByID(context.Context, uuid.UUID) (*domain.Slot, error) {
	return nil, nil
}
func (s *workerSlotRepoStub) ListAvailableByRoomAndDate(context.Context, uuid.UUID, time.Time) ([]*domain.SlotWithStatus, error) {
	return nil, nil
}

func TestNewSlotWorkerAppliesDefaults(t *testing.T) {
	worker := NewSlotWorker(
		&workerScheduleRepoStub{},
		&workerSlotRepoStub{},
		nil,
		0,
		0,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	if worker.builder == nil {
		t.Fatal("expected builder to be initialized")
	}
	if worker.generationDays != 7 {
		t.Fatalf("expected default generation days 7, got %d", worker.generationDays)
	}
	if worker.interval != time.Hour {
		t.Fatalf("expected default interval 1h, got %s", worker.interval)
	}
}

func TestGenerateCreatesSlotsForSchedules(t *testing.T) {
	slotRepo := &workerSlotRepoStub{}
	worker := NewSlotWorker(
		&workerScheduleRepoStub{
			listAllFn: func(context.Context) ([]*domain.Schedule, error) {
				return []*domain.Schedule{{
					ID:         uuid.New(),
					RoomID:     uuid.New(),
					DaysOfWeek: []int16{1, 2, 3, 4, 5, 6, 7},
					StartTime:  time.Date(2000, 1, 1, 10, 0, 0, 0, time.UTC),
					EndTime:    time.Date(2000, 1, 1, 11, 0, 0, 0, time.UTC),
				}}, nil
			},
		},
		slotRepo,
		nil,
		7,
		time.Hour,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	worker.generate(context.Background())

	if slotRepo.bulkCreateCalls != 1 {
		t.Fatalf("expected BulkCreate to be called once, got %d", slotRepo.bulkCreateCalls)
	}
}
