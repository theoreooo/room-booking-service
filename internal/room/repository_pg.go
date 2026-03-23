package room

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"booker/internal/domain"
)

type RoomRepository struct {
	db domain.Querier
}

func NewRoomRepository(db domain.Querier) *RoomRepository {
	return &RoomRepository{db: db}
}

func (r *RoomRepository) Create(ctx context.Context, room *domain.Room) (*domain.Room, error) {
	const query = `
		INSERT INTO rooms (name, description, capacity)
		VALUES ($1, $2, $3)
		RETURNING id, created_at
	`

	err := r.db.QueryRow(ctx, query,
		room.Name,
		room.Description,
		room.Capacity,
	).Scan(
		&room.ID,
		&room.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("room repo: create: %w", err)
	}

	return room, nil
}

func (r *RoomRepository) List(ctx context.Context) ([]*domain.Room, error) {
	const query = `
		SELECT id, name, description, capacity, created_at
		FROM rooms
		ORDER BY created_at DESC
	`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("room repo: list: %w", err)
	}
	defer rows.Close()

	rooms := make([]*domain.Room, 0)

	for rows.Next() {
		room := &domain.Room{}

		if err := rows.Scan(
			&room.ID,
			&room.Name,
			&room.Description,
			&room.Capacity,
			&room.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("room repo: list scan: %w", err)
		}

		rooms = append(rooms, room)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("room repo: rows err: %w", err)
	}

	return rooms, nil
}

func (r *RoomRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Room, error) {
	const query = `
		SELECT id, name, description, capacity, created_at
		FROM rooms
		WHERE id = $1
	`

	room := &domain.Room{}

	err := r.db.QueryRow(ctx, query, id).Scan(
		&room.ID,
		&room.Name,
		&room.Description,
		&room.Capacity,
		&room.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrRoomNotFound
		}
		return nil, fmt.Errorf("room repo: get by id: %w", err)
	}

	return room, nil
}
