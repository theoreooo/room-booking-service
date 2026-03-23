package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"booker/internal/domain"
	"booker/internal/httputil"
)

type contextKey string

const claimsKey contextKey = "claims"

type Claims struct {
	UserID uuid.UUID
	Role   domain.UserRole
}

func ClaimsFromCtx(ctx context.Context) (Claims, bool) {
	c, ok := ctx.Value(claimsKey).(Claims)
	return c, ok
}

func Auth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")

			if header == "" || !strings.HasPrefix(header, "Bearer ") {
				httputil.WriteError(w, http.StatusUnauthorized, domain.ErrUnauthorized)
				return
			}

			tokenStr := strings.TrimPrefix(header, "Bearer ")

			token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
				}
				return []byte(secret), nil
			}, jwt.WithValidMethods([]string{"HS256"}))

			if err != nil || !token.Valid {
				httputil.WriteError(w, http.StatusUnauthorized, domain.ErrUnauthorized)
				return
			}

			mapClaims, ok := token.Claims.(jwt.MapClaims)
			if !ok {
				httputil.WriteError(w, http.StatusUnauthorized, domain.ErrUnauthorized)
				return
			}

			rawID, ok := mapClaims["user_id"].(string)
			if !ok {
				httputil.WriteError(w, http.StatusUnauthorized, domain.ErrUnauthorized)
				return
			}

			userID, err := uuid.Parse(rawID)
			if err != nil {
				httputil.WriteError(w, http.StatusUnauthorized, domain.ErrUnauthorized)
				return
			}

			rawRole, ok := mapClaims["role"].(string)
			if !ok {
				httputil.WriteError(w, http.StatusUnauthorized, domain.ErrUnauthorized)
				return
			}

			role := domain.UserRole(rawRole)
			if role != domain.UserRoleAdmin && role != domain.UserRoleUser {
				httputil.WriteError(w, http.StatusUnauthorized, domain.ErrUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), claimsKey, Claims{
				UserID: userID,
				Role:   role,
			})

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
