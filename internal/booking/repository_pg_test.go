package booking

import (
	"context"
	"errors"
	"testing"

	"booker/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v5"
	pgxpgconn "github.com/jackc/pgx/v5/pgconn"
)

type rowStub struct {
	scanFn func(dest ...any) error
}

func (r rowStub) Scan(dest ...any) error {
	if r.scanFn != nil {
		return r.scanFn(dest...)
	}

	return nil
}

type querierStub struct {
	queryRowFn func(context.Context, string, ...any) pgx.Row
}

func (q querierStub) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, nil
}

func (q querierStub) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if q.queryRowFn != nil {
		return q.queryRowFn(ctx, sql, args...)
	}

	return rowStub{}
}

func (q querierStub) Exec(context.Context, string, ...any) (pgxpgconn.CommandTag, error) {
	return pgxpgconn.CommandTag{}, nil
}

func TestRepositoryCreateReturnsSlotAlreadyBookedWhenConflictDoesNothing(t *testing.T) {
	repo := NewRepository(querierStub{
		queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return rowStub{
				scanFn: func(dest ...any) error {
					return pgx.ErrNoRows
				},
			}
		},
	})

	created, err := repo.Create(context.Background(), &domain.Booking{
		ID:     uuid.New(),
		SlotID: uuid.New(),
		UserID: uuid.New(),
		Status: domain.BookingStatusActive,
	})

	if err != domain.ErrSlotAlreadyBooked {
		t.Fatalf("expected ErrSlotAlreadyBooked, got %v", err)
	}
	if created != nil {
		t.Fatalf("expected nil booking, got %#v", created)
	}
}

func TestRepositoryCreateReturnsSlotNotFoundForSlotForeignKey(t *testing.T) {
	repo := NewRepository(querierStub{
		queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return rowStub{
				scanFn: func(dest ...any) error {
					return &pgconn.PgError{
						Code:           "23503",
						ConstraintName: "bookings_slot_id_fkey",
					}
				},
			}
		},
	})

	created, err := repo.Create(context.Background(), &domain.Booking{
		ID:     uuid.New(),
		SlotID: uuid.New(),
		UserID: uuid.New(),
		Status: domain.BookingStatusActive,
	})

	if err != domain.ErrSlotNotFound {
		t.Fatalf("expected ErrSlotNotFound, got %v", err)
	}
	if created != nil {
		t.Fatalf("expected nil booking, got %#v", created)
	}
}

func TestRepositoryCreateReturnsInternalForUnexpectedError(t *testing.T) {
	repo := NewRepository(querierStub{
		queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return rowStub{
				scanFn: func(dest ...any) error {
					return errors.New("boom")
				},
			}
		},
	})

	created, err := repo.Create(context.Background(), &domain.Booking{
		ID:     uuid.New(),
		SlotID: uuid.New(),
		UserID: uuid.New(),
		Status: domain.BookingStatusActive,
	})

	if err != domain.ErrInternal {
		t.Fatalf("expected ErrInternal, got %v", err)
	}
	if created != nil {
		t.Fatalf("expected nil booking, got %#v", created)
	}
}
