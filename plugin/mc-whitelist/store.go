package mcwhitelist

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"forge.asnk.io/sugar/nekomonogatari-bot/data"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	statePendingAdd    bindingState = "pending_add"
	stateActive        bindingState = "active"
	statePendingDelete bindingState = "pending_delete"
)

var (
	ErrAccountLimit = errors.New("minecraft account limit reached")
	ErrPlayerBound  = errors.New("minecraft player is already bound")
	ErrNotBound     = errors.New("minecraft player is not bound to this user")
	ErrPending      = errors.New("minecraft whitelist operation is pending")
)

type bindingState string

type binding struct {
	ID                int64        `gorm:"primaryKey;autoIncrement"`
	TelegramUserID    int64        `gorm:"not null;uniqueIndex:ux_mc_whitelist_user_slot,priority:1;index:ix_mc_whitelist_user"`
	Slot              uint8        `gorm:"not null;uniqueIndex:ux_mc_whitelist_user_slot,priority:2;check:mc_whitelist_slot,slot >= 1 AND slot <= 2"`
	PlayerName        string       `gorm:"not null"`
	PlayerNameKey     string       `gorm:"not null;uniqueIndex:ux_mc_whitelist_player"`
	State             bindingState `gorm:"type:text;not null;check:mc_whitelist_state,state IN ('pending_add','active','pending_delete')"`
	PreviousState     bindingState `gorm:"type:text;not null;default:'';check:mc_whitelist_previous_state,previous_state IN ('','pending_add','active')"`
	CreatedAtUnix     int64        `gorm:"not null"`
	LastUpdatedAtUnix int64        `gorm:"column:updated_at_unix;not null"`
}

func (binding) TableName() string {
	return "mc-whitelist_players"
}

type playerStore struct {
	db  *gorm.DB
	now func() time.Time
}

func newPlayerStore(ctx context.Context, shared *data.Store) (*playerStore, error) {
	if shared == nil || shared.DB() == nil {
		return nil, errors.New("shared data store must not be nil")
	}
	db := shared.DB()
	if err := db.WithContext(ctx).AutoMigrate(&binding{}); err != nil {
		return nil, fmt.Errorf("migrate %s schema: %w", pluginName, err)
	}
	return &playerStore{db: db, now: time.Now}, nil
}

func (s *playerStore) reserveAdd(ctx context.Context, telegramUserID int64, playerName string) (binding, error) {
	if !validPlayerName(playerName) {
		return binding{}, ErrInvalidPlayerName
	}
	playerKey := strings.ToLower(playerName)
	var reserved binding
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if exists, err := playerExists(tx, playerKey); err != nil {
			return err
		} else if exists {
			return ErrPlayerBound
		}

		now := s.now().Unix()
		for slot := uint8(1); slot <= maxAccountsPerUser; slot++ {
			item := binding{
				TelegramUserID:    telegramUserID,
				Slot:              slot,
				PlayerName:        playerName,
				PlayerNameKey:     playerKey,
				State:             statePendingAdd,
				CreatedAtUnix:     now,
				LastUpdatedAtUnix: now,
			}
			result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&item)
			if result.Error != nil {
				return fmt.Errorf("reserve minecraft binding: %w", result.Error)
			}
			if result.RowsAffected == 1 {
				reserved = item
				return nil
			}
			if exists, err := playerExists(tx, playerKey); err != nil {
				return err
			} else if exists {
				return ErrPlayerBound
			}
		}
		return ErrAccountLimit
	})
	return reserved, err
}

func playerExists(db *gorm.DB, playerKey string) (bool, error) {
	var count int64
	if err := db.Model(&binding{}).Where("player_name_key = ?", playerKey).Count(&count).Error; err != nil {
		return false, fmt.Errorf("check minecraft player binding: %w", err)
	}
	return count != 0, nil
}

func (s *playerStore) activate(ctx context.Context, id int64) error {
	return requireOneRow(s.db.WithContext(ctx).Model(&binding{}).
		Where("id = ? AND state = ?", id, statePendingAdd).
		Updates(map[string]any{
			"state":           stateActive,
			"previous_state":  "",
			"updated_at_unix": s.now().Unix(),
		}), "activate minecraft binding")
}

func (s *playerStore) cancelPendingAdd(ctx context.Context, id int64) error {
	return requireOneRow(s.db.WithContext(ctx).
		Where("id = ? AND state = ?", id, statePendingAdd).
		Delete(&binding{}), "cancel pending minecraft binding")
}

func (s *playerStore) beginDeleteByName(ctx context.Context, telegramUserID int64, playerName string) (binding, error) {
	return s.beginDelete(ctx, telegramUserID, func(tx *gorm.DB, item *binding) error {
		return tx.Where("telegram_user_id = ? AND player_name_key = ?", telegramUserID, strings.ToLower(playerName)).First(item).Error
	})
}

func (s *playerStore) beginDeleteByID(ctx context.Context, telegramUserID, id int64) (binding, error) {
	return s.beginDelete(ctx, telegramUserID, func(tx *gorm.DB, item *binding) error {
		return tx.Where("telegram_user_id = ? AND id = ?", telegramUserID, id).First(item).Error
	})
}

func (s *playerStore) beginDelete(ctx context.Context, telegramUserID int64, find func(*gorm.DB, *binding) error) (binding, error) {
	var item binding
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := find(tx, &item); errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotBound
		} else if err != nil {
			return fmt.Errorf("find minecraft binding for delete: %w", err)
		}
		if item.TelegramUserID != telegramUserID {
			return ErrNotBound
		}
		if item.State == statePendingDelete {
			return ErrPending
		}
		previousState := item.State
		if err := requireOneRow(tx.Model(&binding{}).
			Where("id = ? AND state <> ?", item.ID, statePendingDelete).
			Updates(map[string]any{
				"state":           statePendingDelete,
				"previous_state":  previousState,
				"updated_at_unix": s.now().Unix(),
			}), "reserve minecraft delete"); err != nil {
			return err
		}
		item.PreviousState = previousState
		item.State = statePendingDelete
		return nil
	})
	return item, err
}

func (s *playerStore) finishDelete(ctx context.Context, id int64) error {
	return requireOneRow(s.db.WithContext(ctx).
		Where("id = ? AND state = ?", id, statePendingDelete).
		Delete(&binding{}), "finish minecraft delete")
}

func (s *playerStore) restoreDelete(ctx context.Context, id int64) error {
	var item binding
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ? AND state = ?", id, statePendingDelete).First(&item).Error; err != nil {
			return fmt.Errorf("find pending minecraft delete: %w", err)
		}
		if item.PreviousState != statePendingAdd && item.PreviousState != stateActive {
			return errors.New("pending minecraft delete has no valid previous state")
		}
		return requireOneRow(tx.Model(&binding{}).Where("id = ? AND state = ?", id, statePendingDelete).
			Updates(map[string]any{
				"state":           item.PreviousState,
				"previous_state":  "",
				"updated_at_unix": s.now().Unix(),
			}), "restore minecraft delete")
	})
}

func (s *playerStore) list(ctx context.Context, telegramUserID int64) ([]binding, error) {
	var items []binding
	if err := s.db.WithContext(ctx).Where("telegram_user_id = ?", telegramUserID).Order("id").Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list minecraft bindings: %w", err)
	}
	return items, nil
}

func (s *playerStore) listPending(ctx context.Context) ([]binding, error) {
	var items []binding
	if err := s.db.WithContext(ctx).
		Where("state IN ?", []bindingState{statePendingAdd, statePendingDelete}).
		Order("id").Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list pending minecraft bindings: %w", err)
	}
	return items, nil
}

func (s *playerStore) count(ctx context.Context, telegramUserID int64) (int, error) {
	var count int64
	if err := s.db.WithContext(ctx).Model(&binding{}).Where("telegram_user_id = ?", telegramUserID).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count minecraft bindings: %w", err)
	}
	return int(count), nil
}

func (s *playerStore) byID(ctx context.Context, id int64) (binding, error) {
	var item binding
	if err := s.db.WithContext(ctx).First(&item, id).Error; err != nil {
		return binding{}, fmt.Errorf("read minecraft binding: %w", err)
	}
	return item, nil
}

func requireOneRow(result *gorm.DB, action string) error {
	if result.Error != nil {
		return fmt.Errorf("%s: %w", action, result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("%s changed %d rows, expected 1", action, result.RowsAffected)
	}
	return nil
}
