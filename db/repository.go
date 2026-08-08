package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GameRepository defines decoupled operations for game persistence.
type GameRepository interface {
	Put(ctx context.Context, g Game) error
	GetByID(ctx context.Context, id int) (*Game, error)
	List(ctx context.Context) ([]Game, error)
	SearchByTitle(ctx context.Context, titleSubstr string) ([]Game, error)
	// Clear removes every game. It is the context-aware replacement for the
	// deprecated EmptyCatalogue. Catalogue refresh deliberately does not use
	// it: emptying the catalogue before the replacement data is known loses
	// games whenever a refresh fails or is cancelled part way through.
	Clear(ctx context.Context) error
	// DeleteByIDs removes the games with the given IDs. Passing no IDs is not an error.
	DeleteByIDs(ctx context.Context, ids []int) error
}

// TokenRepository defines decoupled operations for token persistence.
type TokenRepository interface {
	Get(ctx context.Context) (*Token, error)
	Upsert(ctx context.Context, token *Token) error
	// Delete removes the stored token: signing out.
	Delete(ctx context.Context) error
}

// TagRepository is how the marks users put on games are read and written
// without touching the global handle.
type TagRepository interface {
	All(ctx context.Context) (map[int][]string, error)
	Add(ctx context.Context, gameID int, tag string) error
	Remove(ctx context.Context, gameID int, tag string) error
}

// MetadataRepository stores what GOG's store said about games. The payload is
// opaque: its shape belongs to whoever wrote it, and Format names which shape
// that was.
type MetadataRepository interface {
	Get(ctx context.Context, gameID int) (*GameMetadataRecord, error)
	Put(ctx context.Context, gameID, format int, data []byte) error
	All(ctx context.Context) ([]GameMetadataRecord, error)
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
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(&g).Error
}

func (r *gormGameRepo) GetByID(ctx context.Context, id int) (*Game, error) {
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
	var games []Game
	if err := r.db.WithContext(ctx).Find(&games).Error; err != nil {
		return nil, err
	}
	return games, nil
}

func (r *gormGameRepo) SearchByTitle(ctx context.Context, titleSubstr string) ([]Game, error) {
	var games []Game
	if err := r.db.WithContext(ctx).Where("title LIKE ?", "%"+titleSubstr+"%").Find(&games).Error; err != nil {
		return nil, err
	}
	return games, nil
}

func (r *gormGameRepo) Clear(ctx context.Context) error {
	return r.db.WithContext(ctx).Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(&Game{}).Error
}

func (r *gormGameRepo) DeleteByIDs(ctx context.Context, ids []int) error {
	if len(ids) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Unscoped().Delete(&Game{}, "id IN ?", ids).Error
}

func (r *gormTokenRepo) Get(ctx context.Context) (*Token, error) {
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
	token.ID = 1
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"access_token", "refresh_token", "expires_at"}),
	}).Create(token).Error
}

// gormTagRepo is a GORM-backed implementation of TagRepository.
type gormTagRepo struct{ db *gorm.DB }

// NewTagRepository creates a TagRepository. Accepts *gorm.DB to avoid global access.
func NewTagRepository(db *gorm.DB) TagRepository { return &gormTagRepo{db: db} }

func (r *gormTagRepo) All(ctx context.Context) (map[int][]string, error) {
	var rows []GameTag
	if err := r.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to read tags: %w", err)
	}

	tags := make(map[int][]string, len(rows))
	for _, row := range rows {
		tags[row.GameID] = append(tags[row.GameID], row.Tag)
	}
	return tags, nil
}

func (r *gormTagRepo) Add(ctx context.Context, gameID int, tag string) error {
	tag = normalizeTag(tag)
	if tag == "" {
		return fmt.Errorf("a tag cannot be empty")
	}
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).
		Create(&GameTag{GameID: gameID, Tag: tag}).Error
	if err != nil {
		return fmt.Errorf("failed to tag game %d: %w", gameID, err)
	}
	return nil
}

func (r *gormTagRepo) Remove(ctx context.Context, gameID int, tag string) error {
	err := r.db.WithContext(ctx).
		Where("game_id = ? AND tag = ?", gameID, normalizeTag(tag)).
		Delete(&GameTag{}).Error
	if err != nil {
		return fmt.Errorf("failed to untag game %d: %w", gameID, err)
	}
	return nil
}

// gormMetadataRepo is a GORM-backed implementation of MetadataRepository.
type gormMetadataRepo struct{ db *gorm.DB }

// NewMetadataRepository creates a MetadataRepository. Accepts *gorm.DB to
// avoid global access.
func NewMetadataRepository(db *gorm.DB) MetadataRepository { return &gormMetadataRepo{db: db} }

func (r *gormMetadataRepo) Get(ctx context.Context, gameID int) (*GameMetadataRecord, error) {
	var record GameMetadataRecord
	err := r.db.WithContext(ctx).First(&record, "game_id = ?", gameID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata for game %d: %w", gameID, err)
	}
	return &record, nil
}

func (r *gormMetadataRepo) Put(ctx context.Context, gameID, format int, data []byte) error {
	record := GameMetadataRecord{GameID: gameID, Format: format, Data: data, FetchedAt: time.Now()}
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(&record).Error
	if err != nil {
		return fmt.Errorf("failed to store metadata for game %d: %w", gameID, err)
	}
	return nil
}

func (r *gormMetadataRepo) All(ctx context.Context) ([]GameMetadataRecord, error) {
	var records []GameMetadataRecord
	if err := r.db.WithContext(ctx).Find(&records).Error; err != nil {
		return nil, fmt.Errorf("failed to read metadata records: %w", err)
	}
	return records, nil
}

func (r *gormTokenRepo) Delete(ctx context.Context) error {
	if err := r.db.WithContext(ctx).Where("1 = 1").Delete(&Token{}).Error; err != nil {
		return fmt.Errorf("failed to delete token: %w", err)
	}
	return nil
}
