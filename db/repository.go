package db

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GameRepository defines decoupled operations for game persistence.
type GameRepository interface {
	Put(ctx context.Context, g Game) error
	GetByID(ctx context.Context, id int) (*Game, error)
	List(ctx context.Context) ([]Game, error)
	SearchByTitle(ctx context.Context, titleSubstr string) ([]Game, error)
	Clear(ctx context.Context) error
}

// TokenRepository defines decoupled operations for token persistence.
type TokenRepository interface {
	Get(ctx context.Context) (*Token, error)
	Upsert(ctx context.Context, token *Token) error
}

// gormGameRepo is a GORM-backed implementation of GameRepository.
// Use constructor NewGameRepository to obtain an instance.
type gormGameRepo struct{ db *gorm.DB }

// gormTokenRepo is a GORM-backed implementation of TokenRepository.
// Use constructor NewTokenRepository to obtain an instance.
type gormTokenRepo struct{ db *gorm.DB }

// NewGameRepository creates a GameRepository. Accepts *gorm.DB to avoid global access.
func NewGameRepository(db *gorm.DB) GameRepository { return &gormGameRepo{db: db} }

// NewTokenRepository creates a TokenRepository. Accepts *gorm.DB to avoid global access.
func NewTokenRepository(db *gorm.DB) TokenRepository { return &gormTokenRepo{db: db} }

func (r *gormGameRepo) Put(ctx context.Context, g Game) error {
	if r.db == nil {
		return fmt.Errorf("repository not initialized")
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(&g).Error
}

func (r *gormGameRepo) GetByID(ctx context.Context, id int) (*Game, error) {
	if r.db == nil {
		return nil, fmt.Errorf("repository not initialized")
	}
	var game Game
	err := r.db.WithContext(ctx).First(&game, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &game, nil
}

func (r *gormGameRepo) List(ctx context.Context) ([]Game, error) {
	if r.db == nil {
		return nil, fmt.Errorf("repository not initialized")
	}
	var games []Game
	if err := r.db.WithContext(ctx).Find(&games).Error; err != nil {
		return nil, err
	}
	return games, nil
}

func (r *gormGameRepo) SearchByTitle(ctx context.Context, titleSubstr string) ([]Game, error) {
	if r.db == nil {
		return nil, fmt.Errorf("repository not initialized")
	}
	var games []Game
	if err := r.db.WithContext(ctx).Where("title LIKE ?", "%"+titleSubstr+"%").Find(&games).Error; err != nil {
		return nil, err
	}
	return games, nil
}

func (r *gormGameRepo) Clear(ctx context.Context) error {
	if r.db == nil {
		return fmt.Errorf("repository not initialized")
	}
	return r.db.WithContext(ctx).Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(&Game{}).Error
}

func (r *gormTokenRepo) Get(ctx context.Context) (*Token, error) {
	if r.db == nil {
		return nil, fmt.Errorf("repository not initialized")
	}
	var token Token
	err := r.db.WithContext(ctx).First(&token).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &token, nil
}

func (r *gormTokenRepo) Upsert(ctx context.Context, token *Token) error {
	if r.db == nil {
		return fmt.Errorf("repository not initialized")
	}
	token.ID = 1
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"access_token", "refresh_token", "expires_at"}),
	}).Create(token).Error
}
