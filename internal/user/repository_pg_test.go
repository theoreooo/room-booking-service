package user

import (
	"context"
	"errors"
	"testing"

	"booker/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type rowStub struct {
	scanFn func(dest ...any) error
}

func (r rowStub) Scan(dest ...any) error {
	if r.scanFn != nil {
		return r.scanFn(dest...)
	}

	return nil
}

type querierStub struct {
	queryRowFn func(context.Context, string, ...any) pgx.Row
}

func (q querierStub) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, nil
}

func (q querierStub) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if q.queryRowFn != nil {
		return q.queryRowFn(ctx, sql, args...)
	}

	return rowStub{}
}

func (q querierStub) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func TestRepositoryCreateReturnsEmailAlreadyExistsOnUniqueViolation(t *testing.T) {
	repo := NewRepository(querierStub{
		queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return rowStub{
				scanFn: func(dest ...any) error {
					return &pgconn.PgError{
						Code:           "23505",
						ConstraintName: "users_email_key",
					}
				},
			}
		},
	})

	created, err := repo.Create(context.Background(), &domain.User{
		ID:    uuid.New(),
		Email: "user@example.com",
		Role:  domain.UserRoleUser,
	})

	if err != domain.ErrEmailAlreadyExists {
		t.Fatalf("expected ErrEmailAlreadyExists, got %v", err)
	}
	if created != nil {
		t.Fatalf("expected nil user, got %#v", created)
	}
}

func TestRepositoryCreateReturnsInternalForUnexpectedError(t *testing.T) {
	repo := NewRepository(querierStub{
		queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return rowStub{
				scanFn: func(dest ...any) error {
					return errors.New("boom")
				},
			}
		},
	})

	created, err := repo.Create(context.Background(), &domain.User{
		ID:    uuid.New(),
		Email: "user@example.com",
		Role:  domain.UserRoleUser,
	})

	if err != domain.ErrInternal {
		t.Fatalf("expected ErrInternal, got %v", err)
	}
	if created != nil {
		t.Fatalf("expected nil user, got %#v", created)
	}
}
