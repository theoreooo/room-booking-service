package domain

type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string {
	return e.Message
}

var (
	ErrInvalidRequest = &Error{
		Code:    "INVALID_REQUEST",
		Message: "invalid request",
	}

	ErrUnauthorized = &Error{
		Code:    "UNAUTHORIZED",
		Message: "unauthorized",
	}

	ErrForbidden = &Error{
		Code:    "FORBIDDEN",
		Message: "forbidden",
	}

	ErrInvalidCredentials = &Error{
		Code:    "UNAUTHORIZED",
		Message: "invalid email or password",
	}

	ErrEmailAlreadyExists = &Error{
		Code:    "INVALID_REQUEST",
		Message: "email is already taken",
	}

	ErrConferenceUnavailable = &Error{
		Code:    "CONFERENCE_SERVICE_UNAVAILABLE",
		Message: "conference service is unavailable",
	}

	ErrRoomNotFound = &Error{
		Code:    "ROOM_NOT_FOUND",
		Message: "room not found",
	}

	ErrUserNotFound = &Error{
		Code:    "USER_NOT_FOUND",
		Message: "user not found",
	}

	ErrSlotNotFound = &Error{
		Code:    "SLOT_NOT_FOUND",
		Message: "slot not found",
	}

	ErrBookingNotFound = &Error{
		Code:    "BOOKING_NOT_FOUND",
		Message: "booking not found",
	}

	ErrScheduleExists = &Error{
		Code:    "SCHEDULE_EXISTS",
		Message: "schedule for this room already exists and cannot be changed",
	}

	ErrSlotAlreadyBooked = &Error{
		Code:    "SLOT_ALREADY_BOOKED",
		Message: "slot is already booked",
	}

	ErrSlotInThePast = &Error{
		Code:    "INVALID_REQUEST",
		Message: "cannot book a slot in the past",
	}

	ErrNotBookingOwner = &Error{
		Code:    "FORBIDDEN",
		Message: "cannot cancel another user's booking",
	}

	ErrInternal = &Error{
		Code:    "INTERNAL_ERROR",
		Message: "internal server error",
	}
)
