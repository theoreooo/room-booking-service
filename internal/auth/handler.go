package auth

import (
	"booker/internal/config"
	"booker/internal/domain"
	"booker/internal/httputil"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type dummyUserEnsurer interface {
	Ensure(ctx context.Context, user *domain.User) error
}

type userStore interface {
	Create(ctx context.Context, user *domain.User) (*domain.User, error)
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
}

type Handler struct {
	cfg        *config.JWTConfig
	users      userStore
	dummyUsers dummyUserEnsurer
	log        *slog.Logger
}

func NewHTTPHandler(cfg config.JWTConfig, users userStore, dummyUsers dummyUserEnsurer, log *slog.Logger) *Handler {
	return &Handler{
		cfg:        &cfg,
		users:      users,
		dummyUsers: dummyUsers,
		log:        log,
	}
}

type Claims struct {
	UserID uuid.UUID       `json:"user_id"`
	Role   domain.UserRole `json:"role"`
	jwt.RegisteredClaims
}

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type dummyLoginRequest struct {
	Role string `json:"role" enums:"admin,user"`
}

type userResponse struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role" enums:"admin,user"`
	CreatedAt string    `json:"createdAt"`
}

type registerEnvelope struct {
	User userResponse `json:"user"`
}

type tokenResponse struct {
	Token string `json:"token"`
}

// Register godoc
// @Summary Register a user
// @Description Creates a user with email/password and returns created user data.
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body registerRequest true "Register request"
// @Success 201 {object} registerEnvelope
// @Failure 400 {object} httputil.ErrorResponse
// @Failure 500 {object} httputil.ErrorResponse
// @Router /register [post]
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req registerRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, domain.ErrInvalidRequest)
		return
	}

	email, ok := normalizeEmail(req.Email)
	if !ok || strings.TrimSpace(req.Password) == "" {
		httputil.WriteError(w, http.StatusBadRequest, domain.ErrInvalidRequest)
		return
	}

	role := domain.UserRole(req.Role)
	if role != domain.UserRoleAdmin && role != domain.UserRoleUser {
		httputil.WriteError(w, http.StatusBadRequest, domain.ErrInvalidRequest)
		return
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		h.log.Error("failed to hash password", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, domain.ErrInternal)
		return
	}

	passwordHashStr := string(passwordHash)
	user := &domain.User{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: &passwordHashStr,
		Role:         role,
	}

	created, err := h.users.Create(r.Context(), user)
	if err != nil {
		httputil.HandleError(w, h.log, err)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, map[string]any{
		"user": toUserResponse(created),
	})
}

// Login godoc
// @Summary Login by email and password
// @Description Returns JWT token for a registered user.
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body loginRequest true "Login request"
// @Success 200 {object} tokenResponse
// @Failure 400 {object} httputil.ErrorResponse
// @Failure 401 {object} httputil.ErrorResponse
// @Failure 500 {object} httputil.ErrorResponse
// @Router /login [post]
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req loginRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, domain.ErrInvalidRequest)
		return
	}

	email, ok := normalizeEmail(req.Email)
	if !ok || strings.TrimSpace(req.Password) == "" {
		httputil.WriteError(w, http.StatusBadRequest, domain.ErrInvalidRequest)
		return
	}

	user, err := h.users.GetByEmail(r.Context(), email)
	if err != nil {
		if err == domain.ErrUserNotFound {
			httputil.WriteError(w, http.StatusUnauthorized, domain.ErrInvalidCredentials)
			return
		}

		httputil.HandleError(w, h.log, err)
		return
	}

	if user.PasswordHash == nil || bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(req.Password)) != nil {
		httputil.WriteError(w, http.StatusUnauthorized, domain.ErrInvalidCredentials)
		return
	}

	token, err := h.signToken(user.ID, user.Role)
	if err != nil {
		h.log.Error("failed to sign token", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, domain.ErrInternal)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"token": token,
	})
}

// DummyLogin godoc
// @Summary Issue a fixed JWT for a role
// @Description Returns a JWT for the requested role and ensures the dummy user exists in the database.
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body dummyLoginRequest true "Dummy login request"
// @Success 200 {object} tokenResponse
// @Failure 400 {object} httputil.ErrorResponse
// @Failure 500 {object} httputil.ErrorResponse
// @Router /dummyLogin [post]
func (h *Handler) DummyLogin(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req dummyLoginRequest

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

	if err := h.dummyUsers.Ensure(r.Context(), &domain.User{
		ID:    userID,
		Email: dummyUserEmail(role, userID),
		Role:  role,
	}); err != nil {
		h.log.Error("failed to ensure dummy user", "user_id", userID, "role", role, "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, domain.ErrInternal)
		return
	}

	signed, err := h.signToken(userID, role)
	if err != nil {
		h.log.Error("failed to sign dummy token", "err", err)
		httputil.WriteError(w, http.StatusInternalServerError, domain.ErrInternal)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"token": signed,
	})
}

func dummyUserEmail(role domain.UserRole, userID uuid.UUID) string {
	return fmt.Sprintf("dummy-%s-%s@example.local", role, userID.String())
}

func normalizeEmail(email string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(email))
	if normalized == "" {
		return "", false
	}

	addr, err := mail.ParseAddress(normalized)
	if err != nil || addr.Address != normalized {
		return "", false
	}

	return normalized, true
}

func (h *Handler) signToken(userID uuid.UUID, role domain.UserRole) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(h.cfg.AccessTokenTTL)),
		},
	})

	return token.SignedString([]byte(h.cfg.Secret))
}

func toUserResponse(user *domain.User) userResponse {
	return userResponse{
		ID:        user.ID,
		Email:     user.Email,
		Role:      string(user.Role),
		CreatedAt: user.CreatedAt.UTC().Format(time.RFC3339),
	}
}
