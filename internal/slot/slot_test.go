package slot

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

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type slotRoomRepoStub struct {
	getByIDFn func(context.Context, uuid.UUID) (*domain.Room, error)
}

func (s *slotRoomRepoStub) Create(context.Context, *domain.Room) (*domain.Room, error) {
	return nil, nil
}
func (s *slotRoomRepoStub) List(context.Context) ([]*domain.Room, error) {
	return nil, nil
}
func (s *slotRoomRepoStub) GetByID(ctx context.Context, id uuid.UUID) (*domain.Room, error) {
	if s.getByIDFn != nil {
		return s.getByIDFn(ctx, id)
	}
	return nil, nil
}

type slotRepoStubImpl struct {
	bulkCreateFn                 func(context.Context, []*domain.Slot) error
	getByIDFn                    func(context.Context, uuid.UUID) (*domain.Slot, error)
	listAvailableByRoomAndDateFn func(context.Context, uuid.UUID, time.Time) ([]*domain.SlotWithStatus, error)
}

func (s *slotRepoStubImpl) BulkCreate(ctx context.Context, slots []*domain.Slot) error {
	if s.bulkCreateFn != nil {
		return s.bulkCreateFn(ctx, slots)
	}
	return nil
}

func (s *slotRepoStubImpl) GetByID(ctx context.Context, id uuid.UUID) (*domain.Slot, error) {
	if s.getByIDFn != nil {
		return s.getByIDFn(ctx, id)
	}
	return nil, nil
}

func (s *slotRepoStubImpl) ListAvailableByRoomAndDate(ctx context.Context, roomID uuid.UUID, date time.Time) ([]*domain.SlotWithStatus, error) {
	if s.listAvailableByRoomAndDateFn != nil {
		return s.listAvailableByRoomAndDateFn(ctx, roomID, date)
	}
	return nil, nil
}

type slotRowStub struct {
	scanFn func(dest ...any) error
}

func (r slotRowStub) Scan(dest ...any) error {
	if r.scanFn != nil {
		return r.scanFn(dest...)
	}
	return nil
}

type slotQuerierStub struct {
	queryRowFn func(context.Context, string, ...any) pgx.Row
}

func (q slotQuerierStub) Query(context.Context, string, ...any) (pgx.Rows, error) { return nil, nil }
func (q slotQuerierStub) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if q.queryRowFn != nil {
		return q.queryRowFn(ctx, sql, args...)
	}
	return slotRowStub{}
}
func (q slotQuerierStub) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func slotToken(t *testing.T, secret string) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": uuid.NewString(),
		"role":    string(domain.UserRoleUser),
		"exp":     time.Now().Add(time.Hour).Unix(),
	})
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}
	return signed
}

func newSlotTestRouter(secret string, service *Service) http.Handler {
	handler := NewHandler(service, slog.New(slog.NewTextHandler(io.Discard, nil)))
	r := chi.NewRouter()
	r.Use(middleware.Auth(secret))
	handler.Register(r)
	return r
}

func TestBuilderBuildCreatesHalfHourSlots(t *testing.T) {
	builder := NewBuilder()
	schedule := &domain.Schedule{
		RoomID:     uuid.New(),
		DaysOfWeek: []int16{1},
		StartTime:  time.Date(2000, 1, 1, 10, 0, 0, 0, time.UTC),
		EndTime:    time.Date(2000, 1, 1, 11, 0, 0, 0, time.UTC),
	}
	from := time.Date(2026, 3, 23, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 1)

	slots := builder.Build(schedule, from, to)
	if len(slots) != 2 {
		t.Fatalf("expected 2 slots, got %d", len(slots))
	}
	if slots[0].EndAt.Sub(slots[0].StartAt) != 30*time.Minute {
		t.Fatalf("expected 30 minute slot, got %s", slots[0].EndAt.Sub(slots[0].StartAt))
	}
}

func TestServiceListAvailableFiltersBookedSlots(t *testing.T) {
	roomID := uuid.New()
	service := NewService(
		&slotRepoStubImpl{
			listAvailableByRoomAndDateFn: func(context.Context, uuid.UUID, time.Time) ([]*domain.SlotWithStatus, error) {
				return []*domain.SlotWithStatus{
					{Slot: domain.Slot{ID: uuid.New(), RoomID: roomID}},
					{Slot: domain.Slot{ID: uuid.New(), RoomID: roomID}, IsBooked: true},
				}, nil
			},
		},
		&slotRoomRepoStub{
			getByIDFn: func(context.Context, uuid.UUID) (*domain.Room, error) {
				return &domain.Room{ID: roomID}, nil
			},
		},
	)

	slots, err := service.ListAvailable(context.Background(), roomID, time.Date(2026, 3, 25, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(slots) != 1 {
		t.Fatalf("expected 1 available slot, got %d", len(slots))
	}
}

func TestHandlerListAvailableUsesApiYamlFieldNames(t *testing.T) {
	secret := "secret"
	roomID := uuid.New()
	startAt := time.Date(2026, 3, 26, 10, 0, 0, 0, time.UTC)
	service := NewService(
		&slotRepoStubImpl{
			listAvailableByRoomAndDateFn: func(context.Context, uuid.UUID, time.Time) ([]*domain.SlotWithStatus, error) {
				return []*domain.SlotWithStatus{{
					Slot: domain.Slot{
						ID:        uuid.New(),
						RoomID:    roomID,
						StartAt:   startAt,
						EndAt:     startAt.Add(30 * time.Minute),
						CreatedAt: startAt,
					},
				}}, nil
			},
		},
		&slotRoomRepoStub{
			getByIDFn: func(context.Context, uuid.UUID) (*domain.Room, error) {
				return &domain.Room{ID: roomID}, nil
			},
		},
	)

	req := httptest.NewRequest(http.MethodGet, "/rooms/"+roomID.String()+"/slots/list?date=2026-03-26", nil)
	req.Header.Set("Authorization", "Bearer "+slotToken(t, secret))
	rec := httptest.NewRecorder()

	newSlotTestRouter(secret, service).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var body map[string][]map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	slotResp := body["slots"][0]
	if _, ok := slotResp["start"]; !ok {
		t.Fatal("expected start field in response")
	}
	if _, ok := slotResp["end"]; !ok {
		t.Fatal("expected end field in response")
	}
	if _, ok := slotResp["startAt"]; ok {
		t.Fatal("did not expect legacy startAt field in response")
	}
}

func TestHandlerListAvailableRejectsMissingDate(t *testing.T) {
	service := NewService(&slotRepoStubImpl{}, &slotRoomRepoStub{})

	req := httptest.NewRequest(http.MethodGet, "/rooms/"+uuid.NewString()+"/slots/list", nil)
	req.Header.Set("Authorization", "Bearer "+slotToken(t, "secret"))
	rec := httptest.NewRecorder()

	newSlotTestRouter("secret", service).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestRepositoryGetByIDReturnsNotFound(t *testing.T) {
	repo := NewRepository(slotQuerierStub{
		queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return slotRowStub{
				scanFn: func(dest ...any) error {
					return pgx.ErrNoRows
				},
			}
		},
	})

	got, err := repo.GetByID(context.Background(), uuid.New())
	if err != domain.ErrSlotNotFound {
		t.Fatalf("expected ErrSlotNotFound, got %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil slot, got %#v", got)
	}
}

func TestHandlerListAvailableRejectsInvalidRoomID(t *testing.T) {
	service := NewService(&slotRepoStubImpl{}, &slotRoomRepoStub{})

	req := httptest.NewRequest(http.MethodGet, "/rooms/not-a-uuid/slots/list?date=2026-03-26", strings.NewReader(""))
	req.Header.Set("Authorization", "Bearer "+slotToken(t, "secret"))
	rec := httptest.NewRecorder()

	newSlotTestRouter("secret", service).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}
