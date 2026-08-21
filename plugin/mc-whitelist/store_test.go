package mcwhitelist

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"forge.asnk.io/sugar/nekomonogatari-bot/data"
)

func openTestPlayerStore(t *testing.T) *playerStore {
	t.Helper()
	shared, err := data.Open(context.Background(), filepath.Join(t.TempDir(), "mc-whitelist.db"))
	if err != nil {
		t.Fatalf("open data store: %v", err)
	}
	t.Cleanup(func() {
		if err := shared.Close(); err != nil {
			t.Errorf("close data store: %v", err)
		}
	})
	store, err := newPlayerStore(context.Background(), shared)
	if err != nil {
		t.Fatalf("open player store: %v", err)
	}
	store.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	return store
}

func TestPlayerStoreMigrationAndTwoSlotInvariant(t *testing.T) {
	store := openTestPlayerStore(t)
	ctx := context.Background()
	if !store.db.Migrator().HasTable("mc-whitelist_players") {
		t.Fatal("expected mc-whitelist_players table")
	}

	first, err := store.reserveAdd(ctx, 101, "Steve")
	if err != nil {
		t.Fatalf("reserve first player: %v", err)
	}
	second, err := store.reserveAdd(ctx, 101, "Alex_2")
	if err != nil {
		t.Fatalf("reserve second player: %v", err)
	}
	if first.Slot != 1 || second.Slot != 2 {
		t.Fatalf("slots = %d, %d; want 1, 2", first.Slot, second.Slot)
	}
	if _, err := store.reserveAdd(ctx, 101, "ThirdOne"); !errors.Is(err, ErrAccountLimit) {
		t.Fatalf("third reservation error = %v, want ErrAccountLimit", err)
	}
	if _, err := store.reserveAdd(ctx, 202, "sTeVe"); !errors.Is(err, ErrPlayerBound) {
		t.Fatalf("case-insensitive duplicate error = %v, want ErrPlayerBound", err)
	}
	if _, err := store.reserveAdd(ctx, 202, "ThirdOne"); err != nil {
		t.Fatalf("different user reserve: %v", err)
	}
	bypass := binding{
		TelegramUserID:    101,
		Slot:              3,
		PlayerName:        "Bypass",
		PlayerNameKey:     "bypass",
		State:             stateActive,
		CreatedAtUnix:     1,
		LastUpdatedAtUnix: 1,
	}
	if err := store.db.Create(&bypass).Error; err == nil {
		t.Fatal("database accepted slot 3 and allowed the two-account limit to be bypassed")
	}
}

func TestPlayerStoreDeleteOwnershipAndRestore(t *testing.T) {
	store := openTestPlayerStore(t)
	ctx := context.Background()
	item, err := store.reserveAdd(ctx, 101, "Steve")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.activate(ctx, item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.beginDeleteByName(ctx, 202, "Steve"); !errors.Is(err, ErrNotBound) {
		t.Fatalf("other owner delete error = %v, want ErrNotBound", err)
	}
	deleting, err := store.beginDeleteByID(ctx, 101, item.ID)
	if err != nil {
		t.Fatalf("begin delete: %v", err)
	}
	if deleting.State != statePendingDelete || deleting.PreviousState != stateActive {
		t.Fatalf("delete states = %q/%q", deleting.State, deleting.PreviousState)
	}
	if err := store.restoreDelete(ctx, item.ID); err != nil {
		t.Fatalf("restore delete: %v", err)
	}
	restored, err := store.byID(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.State != stateActive || restored.PreviousState != "" {
		t.Fatalf("restored states = %q/%q", restored.State, restored.PreviousState)
	}
	if _, err := store.beginDeleteByName(ctx, 101, "sTEvE"); err != nil {
		t.Fatalf("case-insensitive begin delete: %v", err)
	}
	if err := store.finishDelete(ctx, item.ID); err != nil {
		t.Fatalf("finish delete: %v", err)
	}
	items, err := store.list(ctx, 101)
	if err != nil || len(items) != 0 {
		t.Fatalf("list after delete = %v, %v", items, err)
	}
}

func TestPlayerStoreConcurrentReservationsNeverExceedTwo(t *testing.T) {
	store := openTestPlayerStore(t)
	ctx := context.Background()
	const attempts = 12
	errorsByAttempt := make([]error, attempts)
	var wg sync.WaitGroup
	for index := 0; index < attempts; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			_, errorsByAttempt[index] = store.reserveAdd(ctx, 303, fmt.Sprintf("Player_%d", index))
		}(index)
	}
	wg.Wait()

	items, err := store.list(ctx, 303)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != maxAccountsPerUser {
		t.Fatalf("concurrent reservations stored %d players, want %d; errors=%v", len(items), maxAccountsPerUser, errorsByAttempt)
	}
	for _, item := range items {
		if item.Slot != 1 && item.Slot != 2 {
			t.Fatalf("invalid persisted slot %d", item.Slot)
		}
	}
}

func TestPlayerStoreRejectsInvalidNames(t *testing.T) {
	store := openTestPlayerStore(t)
	for _, name := range []string{"ab", "seventeen_chars_x", "bad name", "bad;op me", "玩家"} {
		if _, err := store.reserveAdd(context.Background(), 404, name); !errors.Is(err, ErrInvalidPlayerName) {
			t.Errorf("reserveAdd(%q) error = %v, want ErrInvalidPlayerName", name, err)
		}
	}
}
