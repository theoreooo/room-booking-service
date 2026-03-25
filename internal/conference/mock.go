package conference

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"booker/internal/config"
	"booker/internal/domain"

	"github.com/google/uuid"
)

type Request struct {
	BookingID uuid.UUID
	SlotID    uuid.UUID
	UserID    uuid.UUID
	StartAt   time.Time
	EndAt     time.Time
}

type MockService struct {
	baseURL    string
	failCreate bool
	failDelete bool
}

func NewMockService(cfg config.ConferenceConfig) *MockService {
	return &MockService{
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		failCreate: cfg.FailCreate,
		failDelete: cfg.FailDelete,
	}
}

func (s *MockService) CreateLink(context.Context, Request) (string, error) {
	if s.failCreate {
		return "", domain.ErrConferenceUnavailable
	}

	link, err := url.JoinPath(s.baseURL, "rooms", uuid.NewString())
	if err != nil {
		return "", domain.ErrInternal
	}

	return fmt.Sprintf("%s?provider=mock", link), nil
}

func (s *MockService) DeleteLink(context.Context, string) error {
	if s.failDelete {
		return domain.ErrInternal
	}

	return nil
}
