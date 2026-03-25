package auth

import (
	"booker/internal/config"
	"booker/internal/domain"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type dummyUserEnsurerStub struct {
	ensureFn    func(context.Context, *domain.User) error
	ensureCalls int
	lastUser    *domain.User
}

func (s *dummyUserEnsurerStub) Ensure(ctx context.Context, user *domain.User) error {
	s.ensureCalls++
	s.lastUser = user

	if s.ensureFn != nil {
		return s.ensureFn(ctx, user)
	}

	return nil
}

func TestDummyLoginEnsuresUserAndReturnsToken(t *testing.T) {
	cfg := config.JWTConfig{
		Secret:         "test-secret",
		AccessTokenTTL: time.Hour,
		AdminUserID:    "00000000-0000-0000-0000-000000000001",
		RegularUserID:  "00000000-0000-0000-0000-000000000002",
	}

	store := &dummyUserEnsurerStub{}
	handler := NewHTTPHandler(cfg, store, slog.New(slog.NewTextHandler(io.Discard, nil)))

	req := httptest.NewRequest(http.MethodPost, "/dummyLogin", strings.NewReader(`{"role":"user"}`))
	rec := httptest.NewRecorder()

	handler.DummyLogin(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if store.ensureCalls != 1 {
		t.Fatalf("expected Ensure to be called once, got %d", store.ensureCalls)
	}

	expectedID := uuid.MustParse(cfg.RegularUserID)
	if store.lastUser == nil {
		t.Fatal("expected Ensure to receive a user")
	}
	if store.lastUser.ID != expectedID {
		t.Fatalf("expected user ID %s, got %s", expectedID, store.lastUser.ID)
	}
	if store.lastUser.Role != domain.UserRoleUser {
		t.Fatalf("expected role %s, got %s", domain.UserRoleUser, store.lastUser.Role)
	}
	if store.lastUser.Email != dummyUserEmail(domain.UserRoleUser, expectedID) {
		t.Fatalf("expected dummy email %q, got %q", dummyUserEmail(domain.UserRoleUser, expectedID), store.lastUser.Email)
	}

	var resp struct {
		Token string `json:"token"`
	}

	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Token == "" {
		t.Fatal("expected token to be returned")
	}

	claims := &Claims{}
	token, err := jwt.ParseWithClaims(resp.Token, claims, func(t *jwt.Token) (any, error) {
		return []byte(cfg.Secret), nil
	})
	if err != nil {
		t.Fatalf("failed to parse token: %v", err)
	}
	if !token.Valid {
		t.Fatal("expected token to be valid")
	}
	if claims.UserID != expectedID {
		t.Fatalf("expected token user ID %s, got %s", expectedID, claims.UserID)
	}
	if claims.Role != domain.UserRoleUser {
		t.Fatalf("expected token role %s, got %s", domain.UserRoleUser, claims.Role)
	}
}

func TestDummyLoginReturnsInternalWhenEnsureFails(t *testing.T) {
	cfg := config.JWTConfig{
		Secret:         "test-secret",
		AccessTokenTTL: time.Hour,
		RegularUserID:  "00000000-0000-0000-0000-000000000002",
	}

	store := &dummyUserEnsurerStub{
		ensureFn: func(context.Context, *domain.User) error {
			return errors.New("db unavailable")
		},
	}

	handler := NewHTTPHandler(cfg, store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodPost, "/dummyLogin", strings.NewReader(`{"role":"user"}`))
	rec := httptest.NewRecorder()

	handler.DummyLogin(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}
}
