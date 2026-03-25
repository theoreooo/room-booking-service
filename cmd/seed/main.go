package main

import (
	"context"
	"log"
	"log/slog"
	"time"

	"booker/internal/booking"
	"booker/internal/conference"
	"booker/internal/config"
	"booker/internal/db"
	"booker/internal/domain"
	"booker/internal/logger"
	"booker/internal/room"
	"booker/internal/schedule"
	"booker/internal/slot"
	"booker/internal/user"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var (
	seedAdminID = uuid.MustParse("10000000-0000-0000-0000-000000000001")
	seedUserID  = uuid.MustParse("10000000-0000-0000-0000-000000000002")

	roomBoardID = uuid.MustParse("20000000-0000-0000-0000-000000000001")
	roomFocusID = uuid.MustParse("20000000-0000-0000-0000-000000000002")
)

type seedRoom struct {
	ID          uuid.UUID
	Name        string
	Description *string
	Capacity    *int
	StartHour   int
	EndHour     int
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	logg := logger.New(cfg.Log.Level)
	ctx := context.Background()

	connectCtx, cancel := context.WithTimeout(ctx, cfg.DB.ConnectTimeout)
	defer cancel()

	pg, err := db.New(connectCtx, cfg.DB)
	if err != nil {
		log.Fatalf("failed to connect to postgres: %v", err)
	}
	defer pg.Close()

	if err := seedData(ctx, cfg, pg, logg); err != nil {
		log.Fatalf("failed to seed data: %v", err)
	}

	logg.Info("seed completed")
}

func seedData(ctx context.Context, cfg *config.Config, pg *db.Storage, logg *slog.Logger) error {
	userRepo := user.NewRepository(pg)
	roomRepo := room.NewRepository(pg)
	scheduleRepo := schedule.NewRepository(pg)
	slotRepo := slot.NewRepository(pg)
	bookingRepo := booking.NewRepository(pg)
	slotBuilder := slot.NewBuilder()

	scheduleService := schedule.NewService(
		scheduleRepo,
		roomRepo,
		slotRepo,
		slotBuilder,
		cfg.Worker.SlotGenerationDays,
		logg,
	)
	bookingService := booking.NewService(bookingRepo, slotRepo, conference.NewMockService(cfg.Conference))

	adminPasswordHash, err := hashPassword("Admin123!")
	if err != nil {
		return err
	}
	userPasswordHash, err := hashPassword("User123!")
	if err != nil {
		return err
	}

	if err := userRepo.Ensure(ctx, &domain.User{
		ID:           seedAdminID,
		Email:        "admin@example.com",
		PasswordHash: &adminPasswordHash,
		Role:         domain.UserRoleAdmin,
	}); err != nil {
		return err
	}

	if err := userRepo.Ensure(ctx, &domain.User{
		ID:           seedUserID,
		Email:        "user@example.com",
		PasswordHash: &userPasswordHash,
		Role:         domain.UserRoleUser,
	}); err != nil {
		return err
	}

	rooms := []seedRoom{
		{
			ID:          roomBoardID,
			Name:        "Board Room",
			Description: ptr("Large room for team syncs"),
			Capacity:    intPtr(8),
			StartHour:   9,
			EndHour:     12,
		},
		{
			ID:          roomFocusID,
			Name:        "Focus Room",
			Description: ptr("Small room for interviews and 1:1s"),
			Capacity:    intPtr(4),
			StartHour:   13,
			EndHour:     17,
		},
	}

	for _, roomSeed := range rooms {
		if err := upsertRoom(ctx, pg, roomSeed); err != nil {
			return err
		}

		if err := ensureScheduleAndSlots(ctx, scheduleRepo, slotRepo, slotBuilder, scheduleService, roomSeed, cfg.Worker.SlotGenerationDays); err != nil {
			return err
		}
	}

	if err := ensureSampleBooking(ctx, slotRepo, bookingService, roomBoardID); err != nil {
		return err
	}

	return nil
}

func upsertRoom(ctx context.Context, pg *db.Storage, roomSeed seedRoom) error {
	query := `
		INSERT INTO rooms (id, name, description, capacity)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE
		SET name = EXCLUDED.name,
		    description = EXCLUDED.description,
		    capacity = EXCLUDED.capacity
	`

	_, err := pg.Exec(ctx, query, roomSeed.ID, roomSeed.Name, roomSeed.Description, roomSeed.Capacity)
	return err
}

func ensureScheduleAndSlots(
	ctx context.Context,
	scheduleRepo *schedule.Repository,
	slotRepo *slot.Repository,
	slotBuilder *slot.Builder,
	scheduleService *schedule.Service,
	roomSeed seedRoom,
	generationDays int,
) error {
	existing, err := scheduleRepo.GetByRoomID(ctx, roomSeed.ID)
	if err != nil {
		return err
	}

	if existing == nil {
		_, err = scheduleService.Create(ctx, &domain.Schedule{
			RoomID:     roomSeed.ID,
			DaysOfWeek: []int16{1, 2, 3, 4, 5, 6, 7},
			StartTime:  time.Date(2000, 1, 1, roomSeed.StartHour, 0, 0, 0, time.UTC),
			EndTime:    time.Date(2000, 1, 1, roomSeed.EndHour, 0, 0, 0, time.UTC),
		})
		return err
	}

	from := time.Now().UTC().Truncate(24 * time.Hour)
	to := from.AddDate(0, 0, generationDays)
	return slotRepo.BulkCreate(ctx, slotBuilder.Build(existing, from, to))
}

func ensureSampleBooking(ctx context.Context, slotRepo *slot.Repository, bookingService *booking.Service, roomID uuid.UUID) error {
	date := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, 1)
	slots, err := slotRepo.ListAvailableByRoomAndDate(ctx, roomID, date)
	if err != nil {
		return err
	}

	targetStart := time.Date(date.Year(), date.Month(), date.Day(), 9, 0, 0, 0, time.UTC)

	for _, slotItem := range slots {
		if !slotItem.StartAt.Equal(targetStart) {
			continue
		}
		if slotItem.IsBooked {
			return nil
		}

		_, err := bookingService.Create(ctx, &domain.Booking{
			SlotID: slotItem.ID,
			UserID: seedUserID,
		}, booking.CreateOptions{CreateConferenceLink: true})
		if err == domain.ErrSlotAlreadyBooked {
			return nil
		}

		return err
	}

	return nil
}

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}

	return string(hash), nil
}

func ptr(value string) *string {
	return &value
}

func intPtr(value int) *int {
	return &value
}
