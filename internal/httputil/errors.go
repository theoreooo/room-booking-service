package httputil

import (
	"booker/internal/domain"
	"errors"
	"log/slog"
	"net/http"
)

func HandleError(w http.ResponseWriter, log *slog.Logger, err error) {
	var domErr *domain.Error

	if errors.As(err, &domErr) {
		status := DomainErrToStatus(domErr)
		WriteError(w, status, domErr)
		return
	}

	log.Error("unexpected error", "err", err)
	WriteError(w, http.StatusInternalServerError, domain.ErrInternal)
}

func DomainErrToStatus(err *domain.Error) int {
	switch err {
	case domain.ErrForbidden, domain.ErrNotBookingOwner:
		return http.StatusForbidden
	case domain.ErrUnauthorized:
		return http.StatusUnauthorized
	case domain.ErrRoomNotFound, domain.ErrSlotNotFound, domain.ErrBookingNotFound:
		return http.StatusNotFound
	case domain.ErrScheduleExists, domain.ErrSlotAlreadyBooked:
		return http.StatusConflict
	case domain.ErrInvalidRequest, domain.ErrSlotInThePast:
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}
