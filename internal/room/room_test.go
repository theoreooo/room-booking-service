package room

import (
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

	"booker/internal/domain"
	"booker/internal/middleware"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type roomRowStub struct {
	scanFn func(dest ...any) error
}

func (r roomRowStub) Scan(dest ...any) error {
	if r.scanFn != nil {
		return r.scanFn(dest...)
	}

	return nil
}

type roomRowsStub struct {
	scanFns   []func(dest ...any) error
	nextIndex int
	scanIndex int
	err       error
}

func (r *roomRowsStub) Close()                                       {}
func (r *roomRowsStub) Err() error                                   { return r.err }
func (r *roomRowsStub) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *roomRowsStub) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *roomRowsStub) Next() bool {
	if r.nextIndex >= len(r.scanFns) {
		return false
	}
	r.nextIndex++
	return true
}
func (r *roomRowsStub) Scan(dest ...any) error {
	idx := r.scanIndex
	r.scanIndex++
	return r.scanFns[idx](dest...)
}
func (r *roomRowsStub) Values() ([]any, error) { return nil, nil }
func (r *roomRowsStub) RawValues() [][]byte    { return nil }
func (r *roomRowsStub) Conn() *pgx.Conn        { return nil }

type roomQuerierStub struct {
	queryFn    func(context.Context, string, ...any) (pgx.Rows, error)
	queryRowFn func(context.Context, string, ...any) pgx.Row
}

func (q roomQuerierStub) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if q.queryFn != nil {
		return q.queryFn(ctx, sql, args...)
	}

	return &roomRowsStub{}, nil
}

func (q roomQuerierStub) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if q.queryRowFn != nil {
		return q.queryRowFn(ctx, sql, args...)
	}

	return roomRowStub{}
}

func (q roomQuerierStub) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

type roomRepoStub struct {
	createFn  func(context.Context, *domain.Room) (*domain.Room, error)
	listFn    func(context.Context) ([]*domain.Room, error)
	getByIDFn func(context.Context, uuid.UUID) (*domain.Room, error)
}

func (s *roomRepoStub) Create(ctx context.Context, room *domain.Room) (*domain.Room, error) {
	if s.createFn != nil {
		return s.createFn(ctx, room)
	}
	return room, nil
}

func (s *roomRepoStub) List(ctx context.Context) ([]*domain.Room, error) {
	if s.listFn != nil {
		return s.listFn(ctx)
	}
	return nil, nil
}

func (s *roomRepoStub) GetByID(ctx context.Context, id uuid.UUID) (*domain.Room, error) {
	if s.getByIDFn != nil {
		return s.getByIDFn(ctx, id)
	}
	return nil, nil
}

func roomToken(t *testing.T, secret string, role domain.UserRole) string {
	t.Helper()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": uuid.NewString(),
		"role":    string(role),
		"exp":     time.Now().Add(time.Hour).Unix(),
	})
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	return signed
}

func newRoomTestRouter(secret string, service *Service) http.Handler {
	handler := NewHandler(service, slog.New(slog.NewTextHandler(io.Discard, nil)))
	r := chi.NewRouter()
	r.Use(middleware.Auth(secret))
	handler.Register(r)
	return r
}

func TestRepositoryCreateScansReturnedValues(t *testing.T) {
	roomID := uuid.New()
	createdAt := time.Date(2026, 3, 25, 10, 0, 0, 0, time.UTC)

	repo := NewRepository(roomQuerierStub{
		queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return roomRowStub{
				scanFn: func(dest ...any) error {
					*(dest[0].(*uuid.UUID)) = roomID
					*(dest[1].(*time.Time)) = createdAt
					return nil
				},
			}
		},
	})

	room := &domain.Room{Name: "Board"}
	created, err := repo.Create(context.Background(), room)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if created.ID != roomID {
		t.Fatalf("expected id %s, got %s", roomID, created.ID)
	}
	if !created.CreatedAt.Equal(createdAt) {
		t.Fatalf("expected createdAt %s, got %s", createdAt, created.CreatedAt)
	}
}

func TestRepositoryListReturnsRooms(t *testing.T) {
	createdAt := time.Date(2026, 3, 25, 10, 0, 0, 0, time.UTC)
	repo := NewRepository(roomQuerierStub{
		queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
			return &roomRowsStub{
				scanFns: []func(dest ...any) error{
					func(dest ...any) error {
						*(dest[0].(*uuid.UUID)) = uuid.MustParse("00000000-0000-0000-0000-000000000011")
						*(dest[1].(*string)) = "Board"
						*(dest[2].(**string)) = nil
						*(dest[3].(**int)) = nil
						*(dest[4].(*time.Time)) = createdAt
						return nil
					},
				},
			}, nil
		},
	})

	rooms, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(rooms) != 1 {
		t.Fatalf("expected 1 room, got %d", len(rooms))
	}
	if rooms[0].Name != "Board" {
		t.Fatalf("expected room name Board, got %s", rooms[0].Name)
	}
}

func TestRepositoryGetByIDReturnsNotFound(t *testing.T) {
	repo := NewRepository(roomQuerierStub{
		queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return roomRowStub{
				scanFn: func(dest ...any) error {
					return pgx.ErrNoRows
				},
			}
		},
	})

	room, err := repo.GetByID(context.Background(), uuid.New())
	if err != domain.ErrRoomNotFound {
		t.Fatalf("expected ErrRoomNotFound, got %v", err)
	}
	if room != nil {
		t.Fatalf("expected nil room, got %#v", room)
	}
}

func TestServiceCreateValidatesInput(t *testing.T) {
	service := NewService(&roomRepoStub{})

	tests := []struct {
		name string
		room *domain.Room
	}{
		{name: "nil room"},
		{name: "empty name", room: &domain.Room{}},
		{name: "invalid capacity", room: &domain.Room{Name: "Board", Capacity: intPtr(0)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			created, err := service.Create(context.Background(), tt.room)
			if err != domain.ErrInvalidRequest {
				t.Fatalf("expected ErrInvalidRequest, got %v", err)
			}
			if created != nil {
				t.Fatalf("expected nil room, got %#v", created)
			}
		})
	}
}

func TestHandlerCreateRequiresAdmin(t *testing.T) {
	secret := "secret"
	service := NewService(&roomRepoStub{})

	req := httptest.NewRequest(http.MethodPost, "/rooms/create", strings.NewReader(`{"name":"Board"}`))
	req.Header.Set("Authorization", "Bearer "+roomToken(t, secret, domain.UserRoleUser))
	rec := httptest.NewRecorder()

	newRoomTestRouter(secret, service).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", rec.Code)
	}
}

func TestHandlerCreateAndList(t *testing.T) {
	secret := "secret"
	createdAt := time.Date(2026, 3, 25, 10, 0, 0, 0, time.UTC)
	repo := &roomRepoStub{
		createFn: func(_ context.Context, room *domain.Room) (*domain.Room, error) {
			room.CreatedAt = createdAt
			return room, nil
		},
		listFn: func(context.Context) ([]*domain.Room, error) {
			return []*domain.Room{{
				ID:        uuid.MustParse("00000000-0000-0000-0000-000000000021"),
				Name:      "Board",
				CreatedAt: createdAt,
			}}, nil
		},
	}
	service := NewService(repo)

	createReq := httptest.NewRequest(http.MethodPost, "/rooms/create", strings.NewReader(`{"name":"Board"}`))
	createReq.Header.Set("Authorization", "Bearer "+roomToken(t, secret, domain.UserRoleAdmin))
	createRec := httptest.NewRecorder()

	newRoomTestRouter(secret, service).ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", createRec.Code)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/rooms/list", nil)
	listReq.Header.Set("Authorization", "Bearer "+roomToken(t, secret, domain.UserRoleUser))
	listRec := httptest.NewRecorder()

	newRoomTestRouter(secret, service).ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", listRec.Code)
	}

	var resp struct {
		Rooms []roomResponse `json:"rooms"`
	}
	if err := json.NewDecoder(listRec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.Rooms) != 1 {
		t.Fatalf("expected 1 room, got %d", len(resp.Rooms))
	}
}

func TestHandlerListHandlesUnexpectedError(t *testing.T) {
	secret := "secret"
	service := NewService(&roomRepoStub{
		listFn: func(context.Context) ([]*domain.Room, error) {
			return nil, errors.New("boom")
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/rooms/list", nil)
	req.Header.Set("Authorization", "Bearer "+roomToken(t, secret, domain.UserRoleUser))
	rec := httptest.NewRecorder()

	newRoomTestRouter(secret, service).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}
}

func intPtr(v int) *int {
	return &v
}
