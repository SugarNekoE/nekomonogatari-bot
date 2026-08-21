package mcwhitelist

import (
	"context"
	"errors"
	"sync"
	"testing"
)

type remoteReply struct {
	outcome remoteOutcome
	err     error
}

type fakeWhitelister struct {
	mu            sync.Mutex
	addReplies    []remoteReply
	removeReplies []remoteReply
	addCalls      []string
	removeCalls   []string
}

func (f *fakeWhitelister) add(_ context.Context, playerName string) (remoteOutcome, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.addCalls = append(f.addCalls, playerName)
	return popRemoteReply(&f.addReplies)
}

func (f *fakeWhitelister) remove(_ context.Context, playerName string) (remoteOutcome, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removeCalls = append(f.removeCalls, playerName)
	return popRemoteReply(&f.removeReplies)
}

func popRemoteReply(replies *[]remoteReply) (remoteOutcome, error) {
	if len(*replies) == 0 {
		return remoteDesired, nil
	}
	reply := (*replies)[0]
	*replies = (*replies)[1:]
	return reply.outcome, reply.err
}

func TestWhitelistServiceSuccessfulLifecycle(t *testing.T) {
	store := openTestPlayerStore(t)
	remote := &fakeWhitelister{}
	service := &whitelistService{store: store, remote: remote}
	ctx := context.Background()

	added, count, err := service.add(ctx, 101, "Steve")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if added.State != stateActive || count != 1 {
		t.Fatalf("add result state/count = %q/%d", added.State, count)
	}
	removed, count, err := service.removeByName(ctx, 101, "sTEvE")
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if removed.PlayerName != "Steve" || count != 0 {
		t.Fatalf("remove result = %#v, count %d", removed, count)
	}
	if len(remote.addCalls) != 1 || remote.addCalls[0] != "Steve" ||
		len(remote.removeCalls) != 1 || remote.removeCalls[0] != "Steve" {
		t.Fatalf("remote calls add=%v remove=%v", remote.addCalls, remote.removeCalls)
	}
}

func TestWhitelistServiceRejectedAddRollsBackReservation(t *testing.T) {
	store := openTestPlayerStore(t)
	remote := &fakeWhitelister{addReplies: []remoteReply{{outcome: remoteRejected, err: errors.New("auth rejected")}}}
	service := &whitelistService{store: store, remote: remote}

	if _, _, err := service.add(context.Background(), 101, "Steve"); !errors.Is(err, ErrRCONUnavailable) {
		t.Fatalf("add error = %v, want ErrRCONUnavailable", err)
	}
	items, err := store.list(context.Background(), 101)
	if err != nil || len(items) != 0 {
		t.Fatalf("bindings after rejected add = %v, %v", items, err)
	}
}

func TestWhitelistServiceUnknownAddPersistsAndReconciles(t *testing.T) {
	store := openTestPlayerStore(t)
	remote := &fakeWhitelister{addReplies: []remoteReply{
		{outcome: remoteUnknown, err: errors.New("response timeout")},
		{outcome: remoteRejected, err: errors.New("temporary auth failure")},
		{outcome: remoteDesired},
	}}
	service := &whitelistService{store: store, remote: remote}

	item, _, err := service.add(context.Background(), 101, "Steve")
	if !errors.Is(err, ErrPending) {
		t.Fatalf("add error = %v, want ErrPending", err)
	}
	persisted, err := store.byID(context.Background(), item.ID)
	if err != nil || persisted.State != statePendingAdd {
		t.Fatalf("pending binding = %#v, %v", persisted, err)
	}
	if err := service.reconcile(context.Background()); err == nil {
		t.Fatal("reconcile with a rejected retry unexpectedly succeeded")
	}
	persisted, err = store.byID(context.Background(), item.ID)
	if err != nil || persisted.State != statePendingAdd {
		t.Fatalf("binding was not preserved after rejected retry = %#v, %v", persisted, err)
	}
	if err := service.reconcile(context.Background()); err != nil {
		t.Fatalf("successful reconcile: %v", err)
	}
	persisted, err = store.byID(context.Background(), item.ID)
	if err != nil || persisted.State != stateActive {
		t.Fatalf("reconciled binding = %#v, %v", persisted, err)
	}
}

func TestWhitelistServiceRejectedAndUnknownDelete(t *testing.T) {
	store := openTestPlayerStore(t)
	remote := &fakeWhitelister{removeReplies: []remoteReply{
		{outcome: remoteRejected, err: errors.New("auth rejected")},
		{outcome: remoteUnknown, err: errors.New("response timeout")},
		{outcome: remoteDesired},
	}}
	service := &whitelistService{store: store, remote: remote}
	ctx := context.Background()
	item, _, err := service.add(ctx, 101, "Steve")
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := service.removeByID(ctx, 101, item.ID); !errors.Is(err, ErrRCONUnavailable) {
		t.Fatalf("rejected remove error = %v", err)
	}
	persisted, err := store.byID(ctx, item.ID)
	if err != nil || persisted.State != stateActive {
		t.Fatalf("binding after rejected remove = %#v, %v", persisted, err)
	}
	if _, _, err := service.removeByID(ctx, 101, item.ID); !errors.Is(err, ErrPending) {
		t.Fatalf("unknown remove error = %v", err)
	}
	persisted, err = store.byID(ctx, item.ID)
	if err != nil || persisted.State != statePendingDelete {
		t.Fatalf("binding after unknown remove = %#v, %v", persisted, err)
	}
	if err := service.reconcile(ctx); err != nil {
		t.Fatalf("reconcile delete: %v", err)
	}
	items, err := store.list(ctx, 101)
	if err != nil || len(items) != 0 {
		t.Fatalf("bindings after reconcile = %v, %v", items, err)
	}
}

func TestWhitelistServiceEnforcesOwnershipAndValidationBeforeRCON(t *testing.T) {
	store := openTestPlayerStore(t)
	remote := &fakeWhitelister{}
	service := &whitelistService{store: store, remote: remote}
	ctx := context.Background()
	item, _, err := service.add(ctx, 101, "Steve")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.removeByID(ctx, 202, item.ID); !errors.Is(err, ErrNotBound) {
		t.Fatalf("other owner remove error = %v", err)
	}
	if _, _, err := service.add(ctx, 101, "bad name"); !errors.Is(err, ErrInvalidPlayerName) {
		t.Fatalf("invalid add error = %v", err)
	}
	if len(remote.addCalls) != 1 || len(remote.removeCalls) != 0 {
		t.Fatalf("unexpected remote calls add=%v remove=%v", remote.addCalls, remote.removeCalls)
	}
}
