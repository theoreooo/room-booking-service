package schedule

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"booker/internal/domain"
	"booker/internal/middleware"
	"booker/internal/slot"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	rootpgconn "github.com/jackc/pgconn"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type scheduleRowStub struct {
	scanFn func(dest ...any) error
}

func (r scheduleRowStub) Scan(dest ...any) error {
	if r.scanFn != nil {
		return r.scanFn(dest...)
	}
	return nil
}

type scheduleRowsStub struct {
	scanFns   []func(dest ...any) error
	nextIndex int
	scanIndex int
	err       error
}

func (r *scheduleRowsStub) Close()                                       {}
func (r *scheduleRowsStub) Err() error                                   { return r.err }
func (r *scheduleRowsStub) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *scheduleRowsStub) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *scheduleRowsStub) Next() bool {
	if r.nextIndex >= len(r.scanFns) {
		return false
	}
	r.nextIndex++
	return true
}
func (r *scheduleRowsStub) Scan(dest ...any) error {
	idx := r.scanIndex
	r.scanIndex++
	return r.scanFns[idx](dest...)
}
func (r *scheduleRowsStub) Values() ([]any, error) { return nil, nil }
func (r *scheduleRowsStub) RawValues() [][]byte    { return nil }
func (r *scheduleRowsStub) Conn() *pgx.Conn        { return nil }

type scheduleQuerierStub struct {
	queryFn    func(context.Context, string, ...any) (pgx.Rows, error)
	queryRowFn func(context.Context, string, ...any) pgx.Row
}

func (q scheduleQuerierStub) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if q.queryFn != nil {
		return q.queryFn(ctx, sql, args...)
	}
	return &scheduleRowsStub{}, nil
}

func (q scheduleQuerierStub) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if q.queryRowFn != nil {
		return q.queryRowFn(ctx, sql, args...)
	}
	return scheduleRowStub{}
}

func (q scheduleQuerierStub) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

type scheduleRepoStub struct {
	createFn      func(context.Context, *domain.Schedule) (*domain.Schedule, error)
	getByRoomIDFn func(context.Context, uuid.UUID) (*domain.Schedule, error)
	listAllFn     func(context.Context) ([]*domain.Schedule, error)
}

func (s *scheduleRepoStub) Create(ctx context.Context, schedule *domain.Schedule) (*domain.Schedule, error) {
	if s.createFn != nil {
		return s.createFn(ctx, schedule)
	}
	return schedule, nil
}

func (s *scheduleRepoStub) GetByRoomID(ctx context.Context, roomID uuid.UUID) (*domain.Schedule, error) {
	if s.getByRoomIDFn != nil {
		return s.getByRoomIDFn(ctx, roomID)
	}
	return nil, nil
}

func (s *scheduleRepoStub) ListAll(ctx context.Context) ([]*domain.Schedule, error) {
	if s.listAllFn != nil {
		return s.listAllFn(ctx)
	}
	return nil, nil
}

type scheduleRoomRepoStub struct {
	getByIDFn func(context.Context, uuid.UUID) (*domain.Room, error)
}

func (s *scheduleRoomRepoStub) Create(context.Context, *domain.Room) (*domain.Room, error) {
	return nil, nil
}
func (s *scheduleRoomRepoStub) List(context.Context) ([]*domain.Room, error) {
	return nil, nil
}
func (s *scheduleRoomRepoStub) GetByID(ctx context.Context, roomID uuid.UUID) (*domain.Room, error) {
	if s.getByIDFn != nil {
		return s.getByIDFn(ctx, roomID)
	}
	return nil, nil
}

type scheduleSlotRepoStub struct {
	bulkCreateFn func(context.Context, []*domain.Slot) error
	lastBulk     []*domain.Slot
}

func (s *scheduleSlotRepoStub) BulkCreate(ctx context.Context, slots []*domain.Slot) error {
	s.lastBulk = slots
	if s.bulkCreateFn != nil {
		return s.bulkCreateFn(ctx, slots)
	}
	return nil
}
func (s *scheduleSlotRepoStub) GetByID(context.Context, uuid.UUID) (*domain.Slot, error) {
	return nil, nil
}
func (s *scheduleSlotRepoStub) ListAvailableByRoomAndDate(context.Context, uuid.UUID, time.Time) ([]*domain.SlotWithStatus, error) {
	return nil, nil
}

func scheduleToken(t *testing.T, secret string, role domain.UserRole) string {
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

func newScheduleTestRouter(secret string, service *Service) http.Handler {
	handler := NewHandler(service, slog.New(slog.NewTextHandler(io.Discard, nil)))
	r := chi.NewRouter()
	r.Use(middleware.Auth(secret))
	handler.Register(r)
	return r
}

func TestRepositoryCreateReturnsScheduleExistsOnUniqueViolation(t *testing.T) {
	repo := NewRepository(scheduleQuerierStub{
		queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return scheduleRowStub{
				scanFn: func(dest ...any) error {
					return &rootpgconn.PgError{Code: "23505"}
				},
			}
		},
	})

	created, err := repo.Create(context.Background(), &domain.Schedule{ID: uuid.New(), RoomID: uuid.New()})
	if err != domain.ErrScheduleExists {
		t.Fatalf("expected ErrScheduleExists, got %v", err)
	}
	if created != nil {
		t.Fatalf("expected nil schedule, got %#v", created)
	}
}

func TestRepositoryGetByRoomIDReturnsNilWhenMissing(t *testing.T) {
	repo := NewRepository(scheduleQuerierStub{
		queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return scheduleRowStub{
				scanFn: func(dest ...any) error {
					return pgx.ErrNoRows
				},
			}
		},
	})

	got, err := repo.GetByRoomID(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil schedule, got %#v", got)
	}
}

func TestServiceCreateValidatesAndCreatesSlots(t *testing.T) {
	roomID := uuid.New()
	slotRepo := &scheduleSlotRepoStub{}
	repo := &scheduleRepoStub{
		createFn: func(_ context.Context, schedule *domain.Schedule) (*domain.Schedule, error) {
			schedule.CreatedAt = time.Date(2026, 3, 25, 10, 0, 0, 0, time.UTC)
			return schedule, nil
		},
	}
	service := NewService(
		repo,
		&scheduleRoomRepoStub{
			getByIDFn: func(context.Context, uuid.UUID) (*domain.Room, error) {
				return &domain.Room{ID: roomID}, nil
			},
		},
		slotRepo,
		slot.NewBuilder(),
		7,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	start, _ := time.Parse("15:04", "10:00")
	end, _ := time.Parse("15:04", "11:00")

	created, err := service.Create(context.Background(), &domain.Schedule{
		RoomID:     roomID,
		DaysOfWeek: []int16{5, 1, 1},
		StartTime:  start,
		EndTime:    end,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if created == nil {
		t.Fatal("expected created schedule")
	}
	if len(created.DaysOfWeek) != 2 || created.DaysOfWeek[0] != 1 || created.DaysOfWeek[1] != 5 {
		t.Fatalf("expected sorted unique days, got %v", created.DaysOfWeek)
	}
	if len(slotRepo.lastBulk) == 0 {
		t.Fatal("expected slots to be generated")
	}
}

func TestServiceCreateReturnsErrorWhenSlotGenerationFails(t *testing.T) {
	roomID := uuid.New()
	repo := &scheduleRepoStub{
		createFn: func(_ context.Context, schedule *domain.Schedule) (*domain.Schedule, error) {
			return schedule, nil
		},
	}
	service := NewService(
		repo,
		&scheduleRoomRepoStub{
			getByIDFn: func(context.Context, uuid.UUID) (*domain.Room, error) {
				return &domain.Room{ID: roomID}, nil
			},
		},
		&scheduleSlotRepoStub{
			bulkCreateFn: func(context.Context, []*domain.Slot) error {
				return domain.ErrInternal
			},
		},
		slot.NewBuilder(),
		7,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	start, _ := time.Parse("15:04", "10:00")
	end, _ := time.Parse("15:04", "11:00")

	created, err := service.Create(context.Background(), &domain.Schedule{
		RoomID:     roomID,
		DaysOfWeek: []int16{1},
		StartTime:  start,
		EndTime:    end,
	})
	if err != domain.ErrInternal {
		t.Fatalf("expected ErrInternal, got %v", err)
	}
	if created != nil {
		t.Fatalf("expected nil schedule, got %#v", created)
	}
}

func TestHandlerCreateRequiresAdmin(t *testing.T) {
	roomID := uuid.New()
	start, _ := time.Parse("15:04", "10:00")
	end, _ := time.Parse("15:04", "11:00")
	service := NewService(
		&scheduleRepoStub{},
		&scheduleRoomRepoStub{
			getByIDFn: func(context.Context, uuid.UUID) (*domain.Room, error) {
				return &domain.Room{ID: roomID}, nil
			},
		},
		&scheduleSlotRepoStub{},
		slot.NewBuilder(),
		7,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	req := httptest.NewRequest(http.MethodPost, "/rooms/"+roomID.String()+"/schedule/create", strings.NewReader(`{"daysOfWeek":[1],"startTime":"`+start.Format("15:04")+`","endTime":"`+end.Format("15:04")+`"}`))
	req.Header.Set("Authorization", "Bearer "+scheduleToken(t, "secret", domain.UserRoleUser))
	rec := httptest.NewRecorder()

	newScheduleTestRouter("secret", service).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", rec.Code)
	}
}

func TestHandlerCreateReturnsSchedule(t *testing.T) {
	roomID := uuid.New()
	service := NewService(
		&scheduleRepoStub{
			createFn: func(_ context.Context, schedule *domain.Schedule) (*domain.Schedule, error) {
				schedule.CreatedAt = time.Date(2026, 3, 25, 10, 0, 0, 0, time.UTC)
				return schedule, nil
			},
		},
		&scheduleRoomRepoStub{
			getByIDFn: func(context.Context, uuid.UUID) (*domain.Room, error) {
				return &domain.Room{ID: roomID}, nil
			},
		},
		&scheduleSlotRepoStub{},
		slot.NewBuilder(),
		7,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	req := httptest.NewRequest(http.MethodPost, "/rooms/"+roomID.String()+"/schedule/create", strings.NewReader(`{"daysOfWeek":[1,3],"startTime":"10:00","endTime":"11:00"}`))
	req.Header.Set("Authorization", "Bearer "+scheduleToken(t, "secret", domain.UserRoleAdmin))
	rec := httptest.NewRecorder()

	newScheduleTestRouter("secret", service).ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", rec.Code)
	}

	var resp struct {
		Schedule scheduleResponse `json:"schedule"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Schedule.RoomID != roomID {
		t.Fatalf("expected room id %s, got %s", roomID, resp.Schedule.RoomID)
	}
}
