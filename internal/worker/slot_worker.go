package worker

import (
	"context"
	"log/slog"
	"time"

	"booker/internal/domain"
	"booker/internal/slot"
)

type SlotWorker struct {
	scheduleRepo   domain.ScheduleRepository
	slotRepo       domain.SlotRepository
	generationDays int
	interval       time.Duration
	logger         *slog.Logger
	builder        *slot.Builder
}

func NewSlotWorker(
	scheduleRepo domain.ScheduleRepository,
	slotRepo domain.SlotRepository,
	builder *slot.Builder,
	generationDays int,
	interval time.Duration,
	logger *slog.Logger,
) *SlotWorker {
	if builder == nil {
		builder = slot.NewBuilder()
	}

	if generationDays < 1 {
		generationDays = 7
	}

	if interval <= 0 {
		interval = time.Hour
	}

	return &SlotWorker{
		scheduleRepo:   scheduleRepo,
		slotRepo:       slotRepo,
		generationDays: generationDays,
		interval:       interval,
		logger:         logger,
		builder:        builder,
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
	end := now.AddDate(0, 0, w.generationDays)

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
