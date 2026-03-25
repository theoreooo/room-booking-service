package httputil

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"booker/internal/domain"
)

func TestDomainErrToStatus(t *testing.T) {
	tests := []struct {
		name string
		err  *domain.Error
		want int
	}{
		{name: "forbidden", err: domain.ErrForbidden, want: http.StatusForbidden},
		{name: "unauthorized", err: domain.ErrUnauthorized, want: http.StatusUnauthorized},
		{name: "not found", err: domain.ErrRoomNotFound, want: http.StatusNotFound},
		{name: "conflict", err: domain.ErrScheduleExists, want: http.StatusConflict},
		{name: "bad request", err: domain.ErrInvalidRequest, want: http.StatusBadRequest},
		{name: "internal", err: domain.ErrInternal, want: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DomainErrToStatus(tt.err); got != tt.want {
				t.Fatalf("expected status %d, got %d", tt.want, got)
			}
		})
	}
}

func TestHandleErrorWritesDomainError(t *testing.T) {
	rec := httptest.NewRecorder()

	HandleError(rec, slog.New(slog.NewTextHandler(io.Discard, nil)), domain.ErrSlotNotFound)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rec.Code)
	}
}

func TestHandleErrorWritesInternalForUnexpectedError(t *testing.T) {
	rec := httptest.NewRecorder()

	HandleError(rec, slog.New(slog.NewTextHandler(io.Discard, nil)), errors.New("boom"))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}
}

func TestWriteJSONAndWriteError(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteJSON(rec, http.StatusCreated, map[string]string{"ok": "true"})

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", rec.Code)
	}

	errRec := httptest.NewRecorder()
	WriteError(errRec, http.StatusBadRequest, domain.ErrInvalidRequest)

	if errRec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", errRec.Code)
	}
	if contentType := errRec.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("expected application/json content type, got %q", contentType)
	}
}
