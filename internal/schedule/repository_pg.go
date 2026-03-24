package schedule

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v5"

	"booker/internal/domain"
)

type Repository struct {
	db domain.Querier
}

func NewRepository(db domain.Querier) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, s *domain.Schedule) (*domain.Schedule, error) {
	query := `
		INSERT INTO schedules (id, room_id, days_of_week, start_time, end_time)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at
	`

	err := r.db.QueryRow(ctx, query,
		s.ID,
		s.RoomID,
		s.DaysOfWeek,
		s.StartTime,
		s.EndTime,
	).Scan(&s.CreatedAt)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case "23505":
				return nil, domain.ErrScheduleExists
			}
		}
		return nil, domain.ErrInternal
	}

	return s, nil
}

func (r *Repository) GetByRoomID(ctx context.Context, roomID uuid.UUID) (*domain.Schedule, error) {
	query := `
		SELECT id, room_id, days_of_week, start_time, end_time, created_at
		FROM schedules
		WHERE room_id = $1
	`

	var s domain.Schedule
	var startTime, endTime time.Time

	err := r.db.QueryRow(ctx, query, roomID).Scan(
		&s.ID,
		&s.RoomID,
		&s.DaysOfWeek,
		&startTime,
		&endTime,
		&s.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, domain.ErrInternal
	}

	s.StartTime = startTime
	s.EndTime = endTime

	return &s, nil
}

func (r *Repository) ListAll(ctx context.Context) ([]*domain.Schedule, error) {
	query := `
		SELECT id, room_id, days_of_week, start_time, end_time, created_at
		FROM schedules
	`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, domain.ErrInternal
	}
	defer rows.Close()

	var result []*domain.Schedule

	for rows.Next() {
		var s domain.Schedule
		var startTime, endTime time.Time

		err := rows.Scan(
			&s.ID,
			&s.RoomID,
			&s.DaysOfWeek,
			&startTime,
			&endTime,
			&s.CreatedAt,
		)
		if err != nil {
			return nil, domain.ErrInternal
		}

		s.StartTime = startTime
		s.EndTime = endTime

		result = append(result, &s)
	}

	return result, nil
}
