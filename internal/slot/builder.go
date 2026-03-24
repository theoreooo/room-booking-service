package slot

import (
	"time"

	"github.com/google/uuid"

	"booker/internal/domain"
)

type Builder struct{}

func NewBuilder() *Builder {
	return &Builder{}
}

func (b *Builder) Build(
	sch *domain.Schedule,
	from, to time.Time,
) []*domain.Slot {

	var result []*domain.Slot

	dayMap := make(map[int]struct{})
	for _, d := range sch.DaysOfWeek {
		dayMap[int(d)] = struct{}{}
	}

	for d := from; d.Before(to); d = d.AddDate(0, 0, 1) {

		weekday := int(d.Weekday())
		if weekday == 0 {
			weekday = 7
		}

		if _, ok := dayMap[weekday]; !ok {
			continue
		}

		start := time.Date(
			d.Year(), d.Month(), d.Day(),
			sch.StartTime.Hour(), sch.StartTime.Minute(), 0, 0,
			time.UTC,
		)

		end := time.Date(
			d.Year(), d.Month(), d.Day(),
			sch.EndTime.Hour(), sch.EndTime.Minute(), 0, 0,
			time.UTC,
		)

		for t := start; t.Before(end); t = t.Add(30 * time.Minute) {
			slotEnd := t.Add(30 * time.Minute)
			if slotEnd.After(end) {
				break
			}

			result = append(result, &domain.Slot{
				ID:      uuid.New(),
				RoomID:  sch.RoomID,
				StartAt: t,
				EndAt:   slotEnd,
			})
		}
	}

	return result
}
