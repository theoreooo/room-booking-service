package slot

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"booker/internal/domain"
)

type Repository struct {
	db domain.Querier
}

func NewRepository(db domain.Querier) *Repository {
	return &Repository{db: db}
}

func (r *Repository) BulkCreate(ctx context.Context, slots []*domain.Slot) error {
	if len(slots) == 0 {
		return nil
	}

	query := `
		INSERT INTO slots (id, room_id, start_at, end_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (room_id, start_at) DO NOTHING
	`

	for _, s := range slots {
		_, err := r.db.Exec(ctx, query,
			s.ID,
			s.RoomID,
			s.StartAt,
			s.EndAt,
		)
		if err != nil {
			return domain.ErrInternal
		}
	}

	return nil
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Slot, error) {
	query := `
		SELECT id, room_id, start_at, end_at, created_at
		FROM slots
		WHERE id = $1
	`

	var s domain.Slot

	err := r.db.QueryRow(ctx, query, id).Scan(
		&s.ID,
		&s.RoomID,
		&s.StartAt,
		&s.EndAt,
		&s.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrSlotNotFound
		}
		return nil, domain.ErrInternal
	}

	return &s, nil
}

func (r *Repository) ListAvailableByRoomAndDate(
	ctx context.Context,
	roomID uuid.UUID,
	date time.Time,
) ([]*domain.SlotWithStatus, error) {

	query := `
		SELECT 
			s.id,
			s.room_id,
			s.start_at,
			s.end_at,
			s.created_at,
			(b.id IS NOT NULL) AS is_booked
		FROM slots s
		LEFT JOIN bookings b 
			ON b.slot_id = s.id AND b.status = 'active'
		WHERE s.room_id = $1
		  AND s.start_at >= $2
		  AND s.start_at < $2 + interval '1 day'
		ORDER BY s.start_at
	`

	rows, err := r.db.Query(ctx, query, roomID, date)
	if err != nil {
		return nil, domain.ErrInternal
	}
	defer rows.Close()

	var result []*domain.SlotWithStatus

	for rows.Next() {
		var s domain.SlotWithStatus

		err := rows.Scan(
			&s.ID,
			&s.RoomID,
			&s.StartAt,
			&s.EndAt,
			&s.CreatedAt,
			&s.IsBooked,
		)
		if err != nil {
			return nil, domain.ErrInternal
		}

		result = append(result, &s)
	}

	if err := rows.Err(); err != nil {
		return nil, domain.ErrInternal
	}

	return result, nil
}
