package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"booker/internal/domain"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func signedToken(t *testing.T, secret string, claims jwt.MapClaims) string {
	t.Helper()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	return signed
}

func TestAuthRejectsMissingBearerToken(t *testing.T) {
	handler := Auth("secret")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("next handler must not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}
}

func TestAuthRejectsInvalidRole(t *testing.T) {
	token := signedToken(t, "secret", jwt.MapClaims{
		"user_id": uuid.NewString(),
		"role":    "guest",
		"exp":     time.Now().Add(time.Hour).Unix(),
	})

	handler := Auth("secret")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("next handler must not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}
}

func TestAuthInjectsClaimsIntoContext(t *testing.T) {
	userID := uuid.New()
	token := signedToken(t, "secret", jwt.MapClaims{
		"user_id": userID.String(),
		"role":    string(domain.UserRoleUser),
		"exp":     time.Now().Add(time.Hour).Unix(),
	})

	var got Claims
	handler := Auth("secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := ClaimsFromCtx(r.Context())
		if !ok {
			t.Fatal("expected claims in context")
		}

		got = claims
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", rec.Code)
	}
	if got.UserID != userID {
		t.Fatalf("expected user id %s, got %s", userID, got.UserID)
	}
	if got.Role != domain.UserRoleUser {
		t.Fatalf("expected role %s, got %s", domain.UserRoleUser, got.Role)
	}
}
