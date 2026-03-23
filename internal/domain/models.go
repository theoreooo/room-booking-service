package domain

import (
	"time"

	"github.com/google/uuid"
)

type UserRole string

const (
	UserRoleAdmin UserRole = "admin"
	UserRoleUser  UserRole = "user"
)

type BookingStatus string

const (
	BookingStatusActive    BookingStatus = "active"
	BookingStatusCancelled BookingStatus = "cancelled"
)

type User struct {
	ID           uuid.UUID `db:"id"`
	Email        string    `db:"email"`
	PasswordHash *string   `db:"password_hash"`
	Role         UserRole  `db:"role"`
	CreatedAt    time.Time `db:"created_at"`
}

type Room struct {
	ID          uuid.UUID `db:"id"`
	Name        string    `db:"name"`
	Description *string   `db:"description"`
	Capacity    *int      `db:"capacity"`
	CreatedAt   time.Time `db:"created_at"`
}

type Schedule struct {
	ID         uuid.UUID `db:"id"`
	RoomID     uuid.UUID `db:"room_id"`
	DaysOfWeek []int     `db:"days_of_week"`
	StartTime  time.Time `db:"start_time"`
	EndTime    time.Time `db:"end_time"`
	CreatedAt  time.Time `db:"created_at"`
}

type Slot struct {
	ID         uuid.UUID `db:"id"`
	RoomID     uuid.UUID `db:"room_id"`
	ScheduleID uuid.UUID `db:"schedule_id"`
	StartAt    time.Time `db:"start_at"`
	EndAt      time.Time `db:"end_at"`
	CreatedAt  time.Time `db:"created_at"`
}

type SlotWithStatus struct {
	Slot
	IsBooked bool `db:"is_booked"`
}

type Booking struct {
	ID             uuid.UUID     `db:"id"`
	SlotID         uuid.UUID     `db:"slot_id"`
	UserID         uuid.UUID     `db:"user_id"`
	Status         BookingStatus `db:"status"`
	ConferenceLink *string       `db:"conference_link"`
	CreatedAt      time.Time     `db:"created_at"`
	UpdatedAt      time.Time     `db:"updated_at"`
}

type BookingWithSlot struct {
	Booking
	Slot Slot `db:"-"`
}
