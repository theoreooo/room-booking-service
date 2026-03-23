package auth

import (
	"booker/internal/config"
	"booker/internal/domain"
	"booker/internal/httputil"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type Handler struct {
	cfg *config.JWTConfig
	log *slog.Logger
}

func NewHTTPHandler(cfg config.JWTConfig, log *slog.Logger) *Handler {
	return &Handler{
		cfg: &cfg,
		log: log,
	}
}

type Claims struct {
	UserID uuid.UUID       `json:"user_id"`
	Role   domain.UserRole `json:"role"`
	jwt.RegisteredClaims
}

func (h *Handler) DummyLogin(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req struct {
		Role string `json:"role"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, domain.ErrInvalidRequest)
		return
	}

	role := domain.UserRole(req.Role)
	if role != domain.UserRoleAdmin && role != domain.UserRoleUser {
		httputil.WriteError(w, http.StatusBadRequest, domain.ErrInvalidRequest)
		return
	}

	var userIDStr string
	if role == domain.UserRoleAdmin {
		userIDStr = h.cfg.AdminUserID
	} else {
		userIDStr = h.cfg.RegularUserID
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		h.log.Error("invalid user id in config", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, domain.ErrInternal)
		return
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(h.cfg.AccessTokenTTL)),
		},
	})

	signed, err := token.SignedString([]byte(h.cfg.Secret))
	if err != nil {
		h.log.Error("failed to sign token", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, domain.ErrInternal)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"token": signed,
	})
}
