package mcwhitelist

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sync"
)

const maxAccountsPerUser = 2

var (
	ErrInvalidPlayerName = errors.New("invalid minecraft player name")
	ErrRCONUnavailable   = errors.New("minecraft RCON operation failed")
)

var playerNamePattern = regexp.MustCompile(`^[A-Za-z0-9_]{3,16}$`)

func validPlayerName(playerName string) bool {
	return playerNamePattern.MatchString(playerName)
}

type whitelistService struct {
	store  *playerStore
	remote whitelister
	mu     sync.Mutex
}

func (s *whitelistService) add(ctx context.Context, telegramUserID int64, playerName string) (binding, int, error) {
	if !validPlayerName(playerName) {
		return binding{}, 0, ErrInvalidPlayerName
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	item, err := s.store.reserveAdd(ctx, telegramUserID, playerName)
	if err != nil {
		return binding{}, 0, err
	}
	outcome, remoteErr := s.remote.add(ctx, item.PlayerName)
	switch outcome {
	case remoteDesired:
		if err := s.store.activate(ctx, item.ID); err != nil {
			return item, 0, fmt.Errorf("%w: persist successful whitelist add: %v", ErrPending, err)
		}
		item.State = stateActive
		count, err := s.store.count(ctx, telegramUserID)
		return item, count, err
	case remoteRejected:
		if err := s.store.cancelPendingAdd(ctx, item.ID); err != nil {
			return item, 0, errors.Join(fmt.Errorf("%w: %v", ErrRCONUnavailable, remoteErr), err)
		}
		return item, 0, fmt.Errorf("%w: %v", ErrRCONUnavailable, remoteErr)
	default:
		return item, 0, fmt.Errorf("%w: %v", ErrPending, remoteErr)
	}
}

func (s *whitelistService) removeByName(ctx context.Context, telegramUserID int64, playerName string) (binding, int, error) {
	if !validPlayerName(playerName) {
		return binding{}, 0, ErrInvalidPlayerName
	}
	return s.remove(ctx, func() (binding, error) {
		return s.store.beginDeleteByName(ctx, telegramUserID, playerName)
	})
}

func (s *whitelistService) removeByID(ctx context.Context, telegramUserID, bindingID int64) (binding, int, error) {
	return s.remove(ctx, func() (binding, error) {
		return s.store.beginDeleteByID(ctx, telegramUserID, bindingID)
	})
}

func (s *whitelistService) remove(ctx context.Context, reserve func() (binding, error)) (binding, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item, err := reserve()
	if err != nil {
		return item, 0, err
	}
	outcome, remoteErr := s.remote.remove(ctx, item.PlayerName)
	switch outcome {
	case remoteDesired:
		if err := s.store.finishDelete(ctx, item.ID); err != nil {
			return item, 0, fmt.Errorf("%w: persist successful whitelist delete: %v", ErrPending, err)
		}
		count, err := s.store.count(ctx, item.TelegramUserID)
		return item, count, err
	case remoteRejected:
		if err := s.store.restoreDelete(ctx, item.ID); err != nil {
			return item, 0, errors.Join(fmt.Errorf("%w: %v", ErrRCONUnavailable, remoteErr), err)
		}
		return item, 0, fmt.Errorf("%w: %v", ErrRCONUnavailable, remoteErr)
	default:
		return item, 0, fmt.Errorf("%w: %v", ErrPending, remoteErr)
	}
}

func (s *whitelistService) list(ctx context.Context, telegramUserID int64) ([]binding, error) {
	return s.store.list(ctx, telegramUserID)
}

func (s *whitelistService) reconcile(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	items, err := s.store.listPending(ctx)
	if err != nil {
		return err
	}
	var reconciliationErrors []error
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(reconciliationErrors, err)...)
		}
		var outcome remoteOutcome
		var remoteErr error
		switch item.State {
		case statePendingAdd:
			outcome, remoteErr = s.remote.add(ctx, item.PlayerName)
			if outcome == remoteDesired {
				err = s.store.activate(ctx, item.ID)
			} else {
				err = pendingRemoteError(remoteErr)
			}
		case statePendingDelete:
			outcome, remoteErr = s.remote.remove(ctx, item.PlayerName)
			if outcome == remoteDesired {
				err = s.store.finishDelete(ctx, item.ID)
			} else {
				err = pendingRemoteError(remoteErr)
			}
		}
		if err != nil {
			reconciliationErrors = append(reconciliationErrors,
				fmt.Errorf("reconcile player %q: %w", item.PlayerName, err))
		}
	}
	return errors.Join(reconciliationErrors...)
}

func pendingRemoteError(err error) error {
	if err != nil {
		return err
	}
	return ErrRCONUnavailable
}
