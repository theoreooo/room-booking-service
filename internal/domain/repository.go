package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

type RoomRepository interface {
	Create(ctx context.Context, room *Room) (*Room, error)
	List(ctx context.Context) ([]*Room, error)
	GetByID(ctx context.Context, id uuid.UUID) (*Room, error)
}

type ScheduleRepository interface {
	Create(ctx context.Context, schedule *Schedule) (*Schedule, error)
	GetByRoomID(ctx context.Context, roomID uuid.UUID) (*Schedule, error)
	ListAll(ctx context.Context) ([]*Schedule, error)
}

type SlotRepository interface {
	BulkCreate(ctx context.Context, slots []*Slot) error
	GetByID(ctx context.Context, id uuid.UUID) (*Slot, error)
	ListAvailableByRoomAndDate(ctx context.Context, roomID uuid.UUID, date time.Time) ([]*SlotWithStatus, error)
}

type BookingRepository interface {
	Create(ctx context.Context, booking *Booking) (*Booking, error)
	GetByID(ctx context.Context, id uuid.UUID) (*Booking, error)
	Cancel(ctx context.Context, id uuid.UUID) (*Booking, error)
	List(ctx context.Context, limit, offset int) ([]*BookingWithSlot, int, error)
	ListByUserFuture(ctx context.Context, userID uuid.UUID, now time.Time) ([]*BookingWithSlot, error)
}

type UserRepository interface {
	Create(ctx context.Context, user *User) (*User, error)
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
}
