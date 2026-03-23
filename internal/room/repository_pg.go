package room

import (
	"booker/internal/domain"
)

type RoomRepository struct {
	db domain.Querier
}

func NewRepository(db domain.Querier) *RoomRepository {
	return &RoomRepository{db: db}
}
