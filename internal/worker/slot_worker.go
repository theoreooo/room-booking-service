package worker

import (
	"context"
	"log/slog"
	"time"

	"booker/internal/domain"
	"booker/internal/slot"
)

type SlotWorker struct {
	scheduleRepo domain.ScheduleRepository
	slotRepo     domain.SlotRepository
	interval     time.Duration
	logger       *slog.Logger
	builder      *slot.Builder
}

func NewSlotWorker(
	scheduleRepo domain.ScheduleRepository,
	slotRepo domain.SlotRepository,
	interval time.Duration,
	logger *slog.Logger,
) *SlotWorker {
	return &SlotWorker{
		scheduleRepo: scheduleRepo,
		slotRepo:     slotRepo,
		interval:     interval,
		logger:       logger,
	}
}

func (w *SlotWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	w.generate(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.generate(ctx)
		}
	}
}

func (w *SlotWorker) generate(ctx context.Context) {
	schedules, err := w.scheduleRepo.ListAll(ctx)
	if err != nil {
		return
	}

	now := time.Now().UTC().Truncate(24 * time.Hour)
	end := now.AddDate(0, 0, 7)

	for _, sch := range schedules {
		slots := w.builder.Build(sch, now, end)
		if len(slots) == 0 {
			continue
		}

		if err := w.slotRepo.BulkCreate(ctx, slots); err != nil {
			w.logger.Error("failed to bulk create slots", "error", err)
		}
	}
}
