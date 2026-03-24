package schedule

import (
	"booker/internal/domain"
	"booker/internal/slot"
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

type Service struct {
	scheduleRepo domain.ScheduleRepository
	roomRepo     domain.RoomRepository
	slotRepo     domain.SlotRepository
	builder      *slot.Builder
	logger       *slog.Logger
}

func NewService(scheduleRepo domain.ScheduleRepository, roomRepo domain.RoomRepository, slotRepo domain.SlotRepository, builder *slot.Builder, logger *slog.Logger) *Service {
	return &Service{scheduleRepo: scheduleRepo, roomRepo: roomRepo, slotRepo: slotRepo, builder: builder, logger: logger}
}

func (s *Service) Create(ctx context.Context, schedule *domain.Schedule) (*domain.Schedule, error) {
	if schedule == nil {
		return nil, domain.ErrInvalidRequest
	}

	_, err := s.roomRepo.GetByID(ctx, schedule.RoomID)
	if err != nil {
		return nil, err
	}

	existing, err := s.scheduleRepo.GetByRoomID(ctx, schedule.RoomID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, domain.ErrScheduleExists
	}

	if len(schedule.DaysOfWeek) == 0 {
		return nil, domain.ErrInvalidRequest
	}

	uniqDays := make(map[int16]struct{})
	for _, d := range schedule.DaysOfWeek {
		if d < 1 || d > 7 {
			return nil, domain.ErrInvalidRequest
		}
		uniqDays[int16(d)] = struct{}{}
	}
	days := make([]int16, 0, len(uniqDays))
	for d := range uniqDays {
		days = append(days, d)
	}

	if !schedule.EndTime.After(schedule.StartTime) {
		return nil, domain.ErrInvalidRequest
	}

	if schedule.EndTime.Sub(schedule.StartTime) < 30*time.Minute {
		return nil, domain.ErrInvalidRequest
	}

	start := time.Date(2000, 1, 1, schedule.StartTime.Hour(), schedule.StartTime.Minute(), 0, 0, time.UTC)
	end := time.Date(2000, 1, 1, schedule.EndTime.Hour(), schedule.EndTime.Minute(), 0, 0, time.UTC)

	toCreateSchedule := &domain.Schedule{
		ID:         uuid.New(),
		RoomID:     schedule.RoomID,
		DaysOfWeek: days,
		StartTime:  start,
		EndTime:    end,
	}

	created, err := s.scheduleRepo.Create(ctx, toCreateSchedule)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC().Truncate(24 * time.Hour)
	end = now.AddDate(0, 0, 7)
	slots := s.builder.Build(created, now, end)
	if err := s.slotRepo.BulkCreate(ctx, slots); err != nil {
		s.logger.Error("failed to bulk create slots", "error", err)
	}

	return created, nil
}
