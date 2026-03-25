package user

import (
	"context"
	"errors"

	"booker/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Repository struct {
	db domain.Querier
}

func NewRepository(db domain.Querier) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Ensure(ctx context.Context, user *domain.User) error {
	if user == nil || user.ID == uuid.Nil || user.Email == "" {
		return domain.ErrInvalidRequest
	}

	query := `
		INSERT INTO users (id, email, password_hash, role)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE
		SET email = EXCLUDED.email,
		    password_hash = EXCLUDED.password_hash,
		    role = EXCLUDED.role
	`

	if _, err := r.db.Exec(ctx, query, user.ID, user.Email, user.PasswordHash, user.Role); err != nil {
		return domain.ErrInternal
	}

	return nil
}

func (r *Repository) Create(ctx context.Context, user *domain.User) (*domain.User, error) {
	query := `
		INSERT INTO users (id, email, password_hash, role)
		VALUES ($1, $2, $3, $4)
		RETURNING created_at
	`

	err := r.db.QueryRow(ctx, query, user.ID, user.Email, user.PasswordHash, user.Role).Scan(&user.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "users_email_key" {
			return nil, domain.ErrEmailAlreadyExists
		}

		return nil, domain.ErrInternal
	}

	return user, nil
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	query := `
		SELECT id, email, password_hash, role, created_at
		FROM users
		WHERE id = $1
	`

	var user domain.User

	err := r.db.QueryRow(ctx, query, id).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.Role,
		&user.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}

		return nil, domain.ErrInternal
	}

	return &user, nil
}

func (r *Repository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	query := `
		SELECT id, email, password_hash, role, created_at
		FROM users
		WHERE email = $1
	`

	var user domain.User

	err := r.db.QueryRow(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.Role,
		&user.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}

		return nil, domain.ErrInternal
	}

	return &user, nil
}
