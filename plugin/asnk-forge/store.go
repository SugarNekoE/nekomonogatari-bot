package asnkforge

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"time"

	"forge.asnk.io/sugar/nekomonogatari-bot/data"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	errPromptExpired     = errors.New("registration prompt is missing or expired")
	errAlreadyRegistered = errors.New("telegram account is already registered")
	errAlreadyClaimed    = errors.New("callback query was already claimed")
	errRegistrationBusy  = errors.New("registration is being processed")
	errNoPending         = errors.New("no pending registration for telegram account")
	errInvalidSession    = errors.New("session is invalid or expired")
	errInvalidCSRF       = errors.New("csrf token is invalid")
)

type forgePromptModel struct {
	ChatID    int64 `gorm:"primaryKey;autoIncrement:false"`
	MessageID int   `gorm:"primaryKey;autoIncrement:false"`
	ThreadID  int   `gorm:"not null;default:0"`
	ExpiresAt int64 `gorm:"not null;index:asnk_forge_prompts_expiry"`
}

func (forgePromptModel) TableName() string { return "asnk-forge_prompts" }

type forgeIntentModel struct {
	ID                 int64  `gorm:"primaryKey;autoIncrement"`
	CallbackQueryID    string `gorm:"not null;uniqueIndex"`
	TelegramUserID     int64  `gorm:"not null;index:asnk_forge_intents_user"`
	TelegramChatID     int64  `gorm:"not null"`
	TelegramThreadID   int    `gorm:"not null;default:0"`
	PromptMessageID    int    `gorm:"not null"`
	TelegramUsername   string `gorm:"not null;default:''"`
	DisplayName        string `gorm:"not null"`
	State              string `gorm:"not null"`
	CreatedAtUnix      int64  `gorm:"column:created_at;not null"`
	ExpiresAt          int64  `gorm:"not null;index:asnk_forge_intents_expiry"`
	AuthenticatedAt    int64
	CompletedAt        int64
	RequestedUsername  string
	RequestedEmailHash []byte `gorm:"size:32"`
	OperationStartedAt int64  `gorm:"not null;default:0"`
	ForgejoUserID      int64
	ForgejoUsername    string
}

func (forgeIntentModel) TableName() string { return "asnk-forge_intents" }

type forgeSessionModel struct {
	SessionDigest []byte           `gorm:"primaryKey;size:32"`
	IntentID      int64            `gorm:"not null;uniqueIndex"`
	CSRFDigest    []byte           `gorm:"not null;size:32"`
	CreatedAtUnix int64            `gorm:"column:created_at;not null"`
	ExpiresAt     int64            `gorm:"not null;index:asnk_forge_sessions_expiry"`
	Intent        forgeIntentModel `gorm:"foreignKey:IntentID;references:ID;constraint:OnDelete:CASCADE"`
}

func (forgeSessionModel) TableName() string { return "asnk-forge_sessions" }

type forgeAccountModel struct {
	TelegramUserID  int64  `gorm:"primaryKey;autoIncrement:false"`
	ForgejoUserID   int64  `gorm:"not null;uniqueIndex"`
	ForgejoUsername string `gorm:"not null;uniqueIndex;collate:nocase"`
	CreatedAtUnix   int64  `gorm:"column:created_at;not null"`
}

func (forgeAccountModel) TableName() string { return "asnk-forge_accounts" }

type forgeStore struct {
	db *gorm.DB
}

type intent struct {
	ID                 int64
	TelegramUserID     int64
	ChatID             int64
	ThreadID           int
	DisplayName        string
	RequestedUsername  string
	RequestedEmailHash []byte
}

func newForgeStore(ctx context.Context, shared *data.Store) (*forgeStore, error) {
	if shared == nil || shared.DB() == nil {
		return nil, errors.New("shared data store is required")
	}
	db := shared.DB().WithContext(ctx)
	if err := db.AutoMigrate(&forgePromptModel{}, &forgeIntentModel{}, &forgeSessionModel{}, &forgeAccountModel{}); err != nil {
		return nil, fmt.Errorf("migrate asnk-forge schema: %w", err)
	}
	if err := db.Exec(`DROP INDEX IF EXISTS "asnk-forge_one_active_intent"`).Error; err != nil {
		return nil, fmt.Errorf("replace asnk-forge active intent index: %w", err)
	}
	if err := db.Exec(`CREATE UNIQUE INDEX "asnk-forge_one_active_intent"
		ON "asnk-forge_intents" (telegram_user_id)
		WHERE state IN ('pending','authenticated','creating','indeterminate','completion_pending')`).Error; err != nil {
		return nil, fmt.Errorf("migrate asnk-forge active intent index: %w", err)
	}
	store := &forgeStore{db: shared.DB()}
	if err := store.reconcileCompletions(ctx, time.Now()); err != nil {
		return nil, err
	}
	if err := store.promoteStaleCreating(ctx, time.Now()); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *forgeStore) recordPrompt(ctx context.Context, chatID int64, messageID, threadID int, expiresAt time.Time) error {
	model := forgePromptModel{ChatID: chatID, MessageID: messageID, ThreadID: threadID, ExpiresAt: expiresAt.Unix()}
	err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "chat_id"}, {Name: "message_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"thread_id", "expires_at"}),
	}).Create(&model).Error
	if err != nil {
		return fmt.Errorf("record forge prompt: %w", err)
	}
	return nil
}

func (s *forgeStore) claim(ctx context.Context, callbackID string, userID, chatID int64, promptMessageID, threadID int, username, displayName string, now time.Time, ttl time.Duration) error {
	recovered := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var prompt forgePromptModel
		if err := tx.Where("chat_id = ? AND message_id = ?", chatID, promptMessageID).First(&prompt).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errPromptExpired
			}
			return fmt.Errorf("read forge prompt: %w", err)
		}
		if prompt.ExpiresAt <= now.Unix() {
			return errPromptExpired
		}

		var pendingCompletion forgeIntentModel
		completionErr := tx.Where("telegram_user_id = ? AND state = ?", userID, "completion_pending").First(&pendingCompletion).Error
		if completionErr == nil {
			if err := finalizeCompletion(tx, &pendingCompletion, now); err != nil {
				return fmt.Errorf("recover completed forge registration: %w", err)
			}
			recovered = true
			return nil
		}
		if !errors.Is(completionErr, gorm.ErrRecordNotFound) {
			return fmt.Errorf("read completed forge registration: %w", completionErr)
		}

		var account forgeAccountModel
		if err := tx.Select("telegram_user_id").Where("telegram_user_id = ?", userID).First(&account).Error; err == nil {
			return errAlreadyRegistered
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("check forge account: %w", err)
		}

		var duplicate forgeIntentModel
		if err := tx.Select("id").Where("callback_query_id = ?", callbackID).First(&duplicate).Error; err == nil {
			return errAlreadyClaimed
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("check forge callback: %w", err)
		}

		if err := tx.Model(&forgeIntentModel{}).
			Where("telegram_user_id = ? AND state IN ? AND expires_at <= ?", userID, []string{"pending", "authenticated"}, now.Unix()).
			Update("state", "expired").Error; err != nil {
			return fmt.Errorf("expire old forge intent: %w", err)
		}

		var active forgeIntentModel
		err := tx.Where("telegram_user_id = ? AND ((state IN ? AND expires_at > ?) OR state IN ?)", userID, []string{"pending", "authenticated"}, now.Unix(), []string{"creating", "indeterminate"}).First(&active).Error
		if err == nil {
			if active.State == "creating" || active.State == "indeterminate" {
				return errRegistrationBusy
			}
			if err := tx.Where("intent_id = ?", active.ID).Delete(&forgeSessionModel{}).Error; err != nil {
				return fmt.Errorf("revoke old forge session: %w", err)
			}
			if err := tx.Model(&forgeIntentModel{}).Where("id = ?", active.ID).Update("state", "cancelled").Error; err != nil {
				return fmt.Errorf("cancel old forge intent: %w", err)
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("read active forge intent: %w", err)
		}

		model := forgeIntentModel{
			CallbackQueryID: callbackID, TelegramUserID: userID, TelegramChatID: chatID,
			TelegramThreadID: threadID, PromptMessageID: promptMessageID, TelegramUsername: username,
			DisplayName: displayName, State: "pending", CreatedAtUnix: now.Unix(), ExpiresAt: now.Add(ttl).Unix(),
		}
		if err := tx.Create(&model).Error; err != nil {
			return fmt.Errorf("insert forge intent: %w", err)
		}
		return nil
	})
	if err == nil && recovered {
		return errAlreadyRegistered
	}
	return err
}

func (s *forgeStore) authenticate(ctx context.Context, userID int64, sessionDigest, csrfDigest []byte, now time.Time, sessionTTL time.Duration) (intent, error) {
	var result intent
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var account forgeAccountModel
		if err := tx.Select("telegram_user_id").Where("telegram_user_id = ?", userID).First(&account).Error; err == nil {
			return errAlreadyRegistered
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("check forge account: %w", err)
		}

		var model forgeIntentModel
		err := tx.Where("telegram_user_id = ? AND state = ? AND expires_at > ?", userID, "pending", now.Unix()).Order("id DESC").First(&model).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			_ = tx.Model(&forgeIntentModel{}).Where("telegram_user_id = ? AND state = ? AND expires_at <= ?", userID, "pending", now.Unix()).Update("state", "expired").Error
			return errNoPending
		}
		if err != nil {
			return fmt.Errorf("read pending forge intent: %w", err)
		}

		expiresAt := now.Add(sessionTTL).Unix()
		operation := tx.Model(&forgeIntentModel{}).Where("id = ? AND state = ?", model.ID, "pending").Updates(map[string]any{"state": "authenticated", "authenticated_at": now.Unix(), "expires_at": expiresAt})
		if operation.Error != nil {
			return fmt.Errorf("authenticate forge intent: %w", operation.Error)
		}
		if operation.RowsAffected != 1 {
			return errNoPending
		}
		session := forgeSessionModel{SessionDigest: sessionDigest, IntentID: model.ID, CSRFDigest: csrfDigest, CreatedAtUnix: now.Unix(), ExpiresAt: expiresAt}
		if err := tx.Omit("Intent").Create(&session).Error; err != nil {
			return fmt.Errorf("create forge session: %w", err)
		}
		result = intentFromModel(model)
		return nil
	})
	return result, err
}

func (s *forgeStore) session(ctx context.Context, sessionDigest []byte, now time.Time) (intent, error) {
	var result intent
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var session forgeSessionModel
		if err := tx.Where("session_digest = ? AND expires_at > ?", sessionDigest, now.Unix()).First(&session).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errInvalidSession
			}
			return fmt.Errorf("read forge session: %w", err)
		}
		var model forgeIntentModel
		if err := tx.Where("id = ? AND expires_at > ? AND state = ?", session.IntentID, now.Unix(), "authenticated").First(&model).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errInvalidSession
			}
			return fmt.Errorf("read forge session intent: %w", err)
		}
		result = intentFromModel(model)
		return nil
	})
	return result, err
}

func (s *forgeStore) rotateCSRF(ctx context.Context, sessionDigest, csrfDigest []byte, now time.Time) error {
	operation := s.db.WithContext(ctx).Model(&forgeSessionModel{}).Where("session_digest = ? AND expires_at > ?", sessionDigest, now.Unix()).Update("csrf_digest", csrfDigest)
	if operation.Error != nil {
		return fmt.Errorf("rotate forge csrf token: %w", operation.Error)
	}
	if operation.RowsAffected != 1 {
		return errInvalidSession
	}
	return nil
}

func (s *forgeStore) revokeSession(ctx context.Context, sessionDigest []byte) error {
	if err := s.db.WithContext(ctx).Where("session_digest = ?", sessionDigest).Delete(&forgeSessionModel{}).Error; err != nil {
		return fmt.Errorf("revoke forge session: %w", err)
	}
	return nil
}

func (s *forgeStore) beginRegistration(ctx context.Context, sessionDigest, csrfDigest []byte, username string, emailHash []byte, now time.Time, operationTTL time.Duration) (intent, error) {
	var result intent
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var session forgeSessionModel
		if err := tx.Where("session_digest = ? AND expires_at > ?", sessionDigest, now.Unix()).First(&session).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errInvalidSession
			}
			return fmt.Errorf("read forge registration session: %w", err)
		}
		if len(session.CSRFDigest) != len(csrfDigest) || subtle.ConstantTimeCompare(session.CSRFDigest, csrfDigest) != 1 {
			return errInvalidCSRF
		}
		var model forgeIntentModel
		if err := tx.Where("id = ? AND state = ? AND expires_at > ?", session.IntentID, "authenticated", now.Unix()).First(&model).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errInvalidSession
			}
			return fmt.Errorf("read forge registration intent: %w", err)
		}
		operation := tx.Model(&forgeIntentModel{}).Where("id = ? AND state = ?", model.ID, "authenticated").Updates(map[string]any{
			"state": "creating", "requested_username": username, "requested_email_hash": emailHash,
			"expires_at": now.Add(operationTTL).Unix(), "operation_started_at": now.Unix(),
		})
		if operation.Error != nil {
			return fmt.Errorf("acquire forge registration: %w", operation.Error)
		}
		if operation.RowsAffected != 1 {
			return errRegistrationBusy
		}
		model.RequestedUsername = username
		model.RequestedEmailHash = append([]byte(nil), emailHash...)
		model.ExpiresAt = now.Add(operationTTL).Unix()
		model.OperationStartedAt = now.Unix()
		result = intentFromModel(model)
		return nil
	})
	return result, err
}

func (s *forgeStore) resetRegistration(ctx context.Context, intentID int64) error {
	operation := s.db.WithContext(ctx).Model(&forgeIntentModel{}).
		Where("id = ? AND state IN ?", intentID, []string{"creating", "indeterminate"}).
		Update("state", "authenticated")
	if operation.Error != nil {
		return fmt.Errorf("reset forge registration: %w", operation.Error)
	}
	if operation.RowsAffected != 1 {
		return errRegistrationBusy
	}
	return nil
}

func (s *forgeStore) failRegistration(ctx context.Context, intentID int64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&forgeIntentModel{}).Where("id = ? AND state IN ?", intentID, []string{"creating", "indeterminate"}).Update("state", "failed").Error; err != nil {
			return fmt.Errorf("fail forge registration: %w", err)
		}
		if err := tx.Where("intent_id = ?", intentID).Delete(&forgeSessionModel{}).Error; err != nil {
			return fmt.Errorf("revoke failed forge session: %w", err)
		}
		return nil
	})
}

func (s *forgeStore) markRegistrationIndeterminate(ctx context.Context, intentID int64) error {
	operation := s.db.WithContext(ctx).Model(&forgeIntentModel{}).
		Where("id = ? AND state = ?", intentID, "creating").
		Update("state", "indeterminate")
	if operation.Error != nil {
		return fmt.Errorf("mark forge registration indeterminate: %w", operation.Error)
	}
	if operation.RowsAffected != 1 {
		return errRegistrationBusy
	}
	return nil
}

func (s *forgeStore) completeRegistration(ctx context.Context, registration intent, user forgejoUser, now time.Time) error {
	operation := s.db.WithContext(ctx).Model(&forgeIntentModel{}).
		Where("id = ? AND state IN ?", registration.ID, []string{"creating", "indeterminate", "completion_pending"}).
		Updates(map[string]any{
			"state": "completion_pending", "forgejo_user_id": user.ID, "forgejo_username": user.Username,
		})
	if operation.Error != nil {
		return fmt.Errorf("record created Forgejo user: %w", operation.Error)
	}
	if operation.RowsAffected != 1 {
		return errRegistrationBusy
	}
	return s.finalizeCompletion(ctx, registration.ID, now)
}

func (s *forgeStore) finalizeCompletion(ctx context.Context, intentID int64, now time.Time) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var model forgeIntentModel
		if err := tx.Where("id = ? AND state = ?", intentID, "completion_pending").First(&model).Error; err != nil {
			return fmt.Errorf("read pending forge completion: %w", err)
		}
		return finalizeCompletion(tx, &model, now)
	})
}

func finalizeCompletion(tx *gorm.DB, model *forgeIntentModel, now time.Time) error {
	account := forgeAccountModel{
		TelegramUserID: model.TelegramUserID, ForgejoUserID: model.ForgejoUserID,
		ForgejoUsername: model.ForgejoUsername, CreatedAtUnix: now.Unix(),
	}
	if err := tx.Create(&account).Error; err != nil {
		return fmt.Errorf("record forge account: %w", err)
	}
	operation := tx.Model(&forgeIntentModel{}).Where("id = ? AND state = ?", model.ID, "completion_pending").Updates(map[string]any{
		"state": "completed", "completed_at": now.Unix(),
	})
	if operation.Error != nil {
		return fmt.Errorf("complete forge intent: %w", operation.Error)
	}
	if operation.RowsAffected != 1 {
		return errRegistrationBusy
	}
	if err := tx.Where("intent_id = ?", model.ID).Delete(&forgeSessionModel{}).Error; err != nil {
		return fmt.Errorf("revoke completed forge session: %w", err)
	}
	return nil
}

func (s *forgeStore) listIndeterminate(ctx context.Context) ([]intent, error) {
	var models []forgeIntentModel
	if err := s.db.WithContext(ctx).Where("state = ?", "indeterminate").Order("id").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list indeterminate forge registrations: %w", err)
	}
	results := make([]intent, 0, len(models))
	for _, model := range models {
		results = append(results, intentFromModel(model))
	}
	return results, nil
}

func (s *forgeStore) promoteStaleCreating(ctx context.Context, staleBefore time.Time) error {
	if err := s.db.WithContext(ctx).Model(&forgeIntentModel{}).
		Where("state = ? AND (operation_started_at IS NULL OR operation_started_at <= ?)", "creating", staleBefore.Unix()).
		Update("state", "indeterminate").Error; err != nil {
		return fmt.Errorf("promote stale forge registrations: %w", err)
	}
	return nil
}

func (s *forgeStore) reconcileCompletions(ctx context.Context, now time.Time) error {
	var models []forgeIntentModel
	if err := s.db.WithContext(ctx).Where("state = ?", "completion_pending").Order("id").Find(&models).Error; err != nil {
		return fmt.Errorf("list pending forge completions: %w", err)
	}
	for _, model := range models {
		if err := s.finalizeCompletion(ctx, model.ID, now); err != nil {
			return fmt.Errorf("reconcile forge completion %d: %w", model.ID, err)
		}
	}
	return nil
}

func (s *forgeStore) cleanupExpired(ctx context.Context, now time.Time) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("expires_at <= ?", now.Unix()).Delete(&forgeSessionModel{}).Error; err != nil {
			return fmt.Errorf("delete expired forge sessions: %w", err)
		}
		if err := tx.Where("expires_at <= ?", now.Unix()).Delete(&forgePromptModel{}).Error; err != nil {
			return fmt.Errorf("delete expired forge prompts: %w", err)
		}
		if err := tx.Model(&forgeIntentModel{}).
			Where("state IN ? AND expires_at <= ?", []string{"pending", "authenticated"}, now.Unix()).
			Update("state", "expired").Error; err != nil {
			return fmt.Errorf("expire abandoned forge intents: %w", err)
		}
		if err := tx.Where("state IN ? AND expires_at <= ?", []string{"cancelled", "expired", "failed"}, now.Unix()).Delete(&forgeIntentModel{}).Error; err != nil {
			return fmt.Errorf("delete expired forge intents: %w", err)
		}
		return nil
	})
}

func intentFromModel(model forgeIntentModel) intent {
	return intent{
		ID: model.ID, TelegramUserID: model.TelegramUserID, ChatID: model.TelegramChatID,
		ThreadID: model.TelegramThreadID, DisplayName: model.DisplayName,
		RequestedUsername: model.RequestedUsername, RequestedEmailHash: append([]byte(nil), model.RequestedEmailHash...),
	}
}
