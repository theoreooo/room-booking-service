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
	"golang.org/x/crypto/bcrypt"
)

type userStoreStub struct {
	ensureFn     func(context.Context, *domain.User) error
	createFn     func(context.Context, *domain.User) (*domain.User, error)
	getByEmailFn func(context.Context, string) (*domain.User, error)
	ensureCalls  int
	createCalls  int
	getCalls     int
	lastUser     *domain.User
	lastEmail    string
}

func (s *userStoreStub) Ensure(ctx context.Context, user *domain.User) error {
	s.ensureCalls++
	s.lastUser = user

	if s.ensureFn != nil {
		return s.ensureFn(ctx, user)
	}

	return nil
}

func (s *userStoreStub) Create(ctx context.Context, user *domain.User) (*domain.User, error) {
	s.createCalls++
	s.lastUser = user

	if s.createFn != nil {
		return s.createFn(ctx, user)
	}

	return user, nil
}

func (s *userStoreStub) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	s.getCalls++
	s.lastEmail = email

	if s.getByEmailFn != nil {
		return s.getByEmailFn(ctx, email)
	}

	return nil, nil
}

func newHandlerForTest(cfg config.JWTConfig, store *userStoreStub) *Handler {
	return NewHTTPHandler(cfg, store, store, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestDummyLoginEnsuresUserAndReturnsToken(t *testing.T) {
	cfg := config.JWTConfig{
		Secret:         "test-secret",
		AccessTokenTTL: time.Hour,
		AdminUserID:    "00000000-0000-0000-0000-000000000001",
		RegularUserID:  "00000000-0000-0000-0000-000000000002",
	}

	store := &userStoreStub{}
	handler := newHandlerForTest(cfg, store)

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

func TestRegisterCreatesUserWithHashedPassword(t *testing.T) {
	cfg := config.JWTConfig{
		Secret:         "test-secret",
		AccessTokenTTL: time.Hour,
	}

	store := &userStoreStub{
		createFn: func(_ context.Context, user *domain.User) (*domain.User, error) {
			user.CreatedAt = time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)
			return user, nil
		},
	}

	handler := newHandlerForTest(cfg, store)
	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(`{"email":" USER@example.com ","password":"secret123","role":"user"}`))
	rec := httptest.NewRecorder()

	handler.Register(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", rec.Code)
	}
	if store.createCalls != 1 {
		t.Fatalf("expected Create to be called once, got %d", store.createCalls)
	}
	if store.lastUser == nil {
		t.Fatal("expected created user to be passed to repository")
	}
	if store.lastUser.Email != "user@example.com" {
		t.Fatalf("expected normalized email, got %q", store.lastUser.Email)
	}
	if store.lastUser.PasswordHash == nil {
		t.Fatal("expected password hash to be set")
	}
	if *store.lastUser.PasswordHash == "secret123" {
		t.Fatal("expected password to be hashed")
	}
	if bcrypt.CompareHashAndPassword([]byte(*store.lastUser.PasswordHash), []byte("secret123")) != nil {
		t.Fatal("expected password hash to match original password")
	}
}

func TestRegisterReturnsBadRequestForDuplicateEmail(t *testing.T) {
	cfg := config.JWTConfig{Secret: "test-secret", AccessTokenTTL: time.Hour}
	store := &userStoreStub{
		createFn: func(context.Context, *domain.User) (*domain.User, error) {
			return nil, domain.ErrEmailAlreadyExists
		},
	}

	handler := newHandlerForTest(cfg, store)
	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(`{"email":"user@example.com","password":"secret123","role":"user"}`))
	rec := httptest.NewRecorder()

	handler.Register(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestLoginReturnsTokenForValidCredentials(t *testing.T) {
	cfg := config.JWTConfig{
		Secret:         "test-secret",
		AccessTokenTTL: time.Hour,
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("failed to generate password hash: %v", err)
	}

	expectedID := uuid.New()
	store := &userStoreStub{
		getByEmailFn: func(_ context.Context, email string) (*domain.User, error) {
			return &domain.User{
				ID:           expectedID,
				Email:        email,
				PasswordHash: ptr(string(passwordHash)),
				Role:         domain.UserRoleAdmin,
			}, nil
		},
	}

	handler := newHandlerForTest(cfg, store)
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"email":" ADMIN@example.com ","password":"secret123"}`))
	rec := httptest.NewRecorder()

	handler.Login(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if store.lastEmail != "admin@example.com" {
		t.Fatalf("expected normalized lookup email, got %q", store.lastEmail)
	}

	var resp struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
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
	if claims.Role != domain.UserRoleAdmin {
		t.Fatalf("expected token role %s, got %s", domain.UserRoleAdmin, claims.Role)
	}
}

func TestLoginRejectsInvalidCredentials(t *testing.T) {
	cfg := config.JWTConfig{
		Secret:         "test-secret",
		AccessTokenTTL: time.Hour,
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("failed to generate password hash: %v", err)
	}

	store := &userStoreStub{
		getByEmailFn: func(_ context.Context, email string) (*domain.User, error) {
			return &domain.User{
				ID:           uuid.New(),
				Email:        email,
				PasswordHash: ptr(string(passwordHash)),
				Role:         domain.UserRoleUser,
			}, nil
		},
	}

	handler := newHandlerForTest(cfg, store)
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"email":"user@example.com","password":"wrong"}`))
	rec := httptest.NewRecorder()

	handler.Login(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}
}

func TestDummyLoginReturnsInternalWhenEnsureFails(t *testing.T) {
	cfg := config.JWTConfig{
		Secret:         "test-secret",
		AccessTokenTTL: time.Hour,
		RegularUserID:  "00000000-0000-0000-0000-000000000002",
	}

	store := &userStoreStub{
		ensureFn: func(context.Context, *domain.User) error {
			return errors.New("db unavailable")
		},
	}

	handler := newHandlerForTest(cfg, store)
	req := httptest.NewRequest(http.MethodPost, "/dummyLogin", strings.NewReader(`{"role":"user"}`))
	rec := httptest.NewRecorder()

	handler.DummyLogin(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}
}

func ptr(value string) *string {
	return &value
}
