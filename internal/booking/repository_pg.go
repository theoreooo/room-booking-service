package booking

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"booker/internal/domain"
)

type Repository struct {
	db domain.Querier
}

func NewRepository(db domain.Querier) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, booking *domain.Booking) (*domain.Booking, error) {
	query := `
		INSERT INTO bookings (id, slot_id, user_id, status, conference_link)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (slot_id) WHERE status = 'active' DO NOTHING
		RETURNING created_at, updated_at
	`

	err := r.db.QueryRow(ctx, query,
		booking.ID,
		booking.SlotID,
		booking.UserID,
		booking.Status,
		booking.ConferenceLink,
	).Scan(
		&booking.CreatedAt,
		&booking.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrSlotAlreadyBooked
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case "23503":
				if pgErr.ConstraintName == "bookings_slot_id_fkey" {
					return nil, domain.ErrSlotNotFound
				}
			}
		}

		return nil, domain.ErrInternal
	}

	return booking, nil
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Booking, error) {
	query := `
		SELECT id, slot_id, user_id, status, conference_link, created_at, updated_at
		FROM bookings
		WHERE id = $1
	`

	var booking domain.Booking

	err := r.db.QueryRow(ctx, query, id).Scan(
		&booking.ID,
		&booking.SlotID,
		&booking.UserID,
		&booking.Status,
		&booking.ConferenceLink,
		&booking.CreatedAt,
		&booking.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrBookingNotFound
		}

		return nil, domain.ErrInternal
	}

	return &booking, nil
}

func (r *Repository) Cancel(ctx context.Context, id uuid.UUID) (*domain.Booking, error) {
	query := `
		UPDATE bookings
		SET status = $2, updated_at = now()
		WHERE id = $1
		RETURNING id, slot_id, user_id, status, conference_link, created_at, updated_at
	`

	var booking domain.Booking

	err := r.db.QueryRow(ctx, query, id, domain.BookingStatusCancelled).Scan(
		&booking.ID,
		&booking.SlotID,
		&booking.UserID,
		&booking.Status,
		&booking.ConferenceLink,
		&booking.CreatedAt,
		&booking.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrBookingNotFound
		}

		return nil, domain.ErrInternal
	}

	return &booking, nil
}

func (r *Repository) List(ctx context.Context, limit, offset int) ([]*domain.BookingWithSlot, int, error) {
	query := `
		SELECT
			b.id,
			b.slot_id,
			b.user_id,
			b.status,
			b.conference_link,
			b.created_at,
			b.updated_at,
			s.id,
			s.room_id,
			s.start_at,
			s.end_at,
			s.created_at,
			COUNT(*) OVER()
		FROM bookings b
		JOIN slots s ON s.id = b.slot_id
		ORDER BY b.created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.db.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, domain.ErrInternal
	}
	defer rows.Close()

	var result []*domain.BookingWithSlot
	total := 0

	for rows.Next() {
		var booking domain.BookingWithSlot
		var totalCount int

		err := rows.Scan(
			&booking.ID,
			&booking.SlotID,
			&booking.UserID,
			&booking.Status,
			&booking.ConferenceLink,
			&booking.CreatedAt,
			&booking.UpdatedAt,
			&booking.Slot.ID,
			&booking.Slot.RoomID,
			&booking.Slot.StartAt,
			&booking.Slot.EndAt,
			&booking.Slot.CreatedAt,
			&totalCount,
		)
		if err != nil {
			return nil, 0, domain.ErrInternal
		}

		total = totalCount
		result = append(result, &booking)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, domain.ErrInternal
	}

	return result, total, nil
}

func (r *Repository) ListByUserFuture(ctx context.Context, userID uuid.UUID, now time.Time) ([]*domain.BookingWithSlot, error) {
	query := `
		SELECT
			b.id,
			b.slot_id,
			b.user_id,
			b.status,
			b.conference_link,
			b.created_at,
			b.updated_at,
			s.id,
			s.room_id,
			s.start_at,
			s.end_at,
			s.created_at
		FROM bookings b
		JOIN slots s ON s.id = b.slot_id
		WHERE b.user_id = $1
		  AND s.start_at >= $2
		ORDER BY s.start_at ASC, b.created_at ASC
	`

	rows, err := r.db.Query(ctx, query, userID, now.UTC())
	if err != nil {
		return nil, domain.ErrInternal
	}
	defer rows.Close()

	var result []*domain.BookingWithSlot

	for rows.Next() {
		var booking domain.BookingWithSlot

		err := rows.Scan(
			&booking.ID,
			&booking.SlotID,
			&booking.UserID,
			&booking.Status,
			&booking.ConferenceLink,
			&booking.CreatedAt,
			&booking.UpdatedAt,
			&booking.Slot.ID,
			&booking.Slot.RoomID,
			&booking.Slot.StartAt,
			&booking.Slot.EndAt,
			&booking.Slot.CreatedAt,
		)
		if err != nil {
			return nil, domain.ErrInternal
		}

		result = append(result, &booking)
	}

	if err := rows.Err(); err != nil {
		return nil, domain.ErrInternal
	}

	return result, nil
}
