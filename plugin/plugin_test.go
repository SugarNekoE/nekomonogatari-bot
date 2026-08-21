package plugin

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot"
)

func TestRegistryRunsRegistrationCallbacksInOrder(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	var got []string
	for _, name := range []string{"one", "two"} {
		name := name
		err := registry.Add(name, Callbacks{Register: func(*bot.Bot) error {
			got = append(got, name)
			return nil
		}})
		if err != nil {
			t.Fatalf("Add(%q) error = %v", name, err)
		}
	}
	if err := registry.RegisterHandlers(nil); err != nil {
		t.Fatalf("RegisterHandlers() error = %v", err)
	}
	if !reflect.DeepEqual(got, []string{"one", "two"}) {
		t.Fatalf("registration order = %v", got)
	}
	if !reflect.DeepEqual(registry.Names(), []string{"one", "two"}) {
		t.Fatalf("Names() = %v", registry.Names())
	}
}

func TestRegistryRejectsInvalidEntries(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	callback := Callbacks{Register: func(*bot.Bot) error { return nil }}
	if err := registry.Add("", callback); err == nil {
		t.Fatal("Add(empty name) succeeded")
	}
	if err := registry.Add("one", Callbacks{}); err == nil {
		t.Fatal("Add(nil callback) succeeded")
	}
	if err := registry.Add("one", callback); err != nil {
		t.Fatal(err)
	}
	if err := registry.Add("one", callback); err == nil {
		t.Fatal("Add(duplicate) succeeded")
	}
}

func TestRegistryWrapsCallbackErrors(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	want := errors.New("register failed")
	if err := registry.Add("broken", Callbacks{Register: func(*bot.Bot) error { return want }}); err != nil {
		t.Fatal(err)
	}
	err := registry.RegisterHandlers(nil)
	if !errors.Is(err, want) || !strings.Contains(err.Error(), "broken") {
		t.Fatalf("RegisterHandlers() error = %v", err)
	}
}

func TestRegistryReportsRunnerErrors(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	want := errors.New("listen failed")
	err := registry.Add("server", Callbacks{
		Register: func(*bot.Bot) error { return nil },
		Run:      func(context.Context) error { return want },
	})
	if err != nil {
		t.Fatal(err)
	}
	registry.Start(context.Background())
	select {
	case got := <-registry.Errors():
		if !errors.Is(got, want) || !strings.Contains(got.Error(), "server") {
			t.Fatalf("runner error = %v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("runner error was not reported")
	}
	registry.Wait()
}

func TestRegistryIgnoresRunnerResultAfterCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	registry := NewRegistry()
	err := registry.Add("server", Callbacks{
		Register: func(*bot.Bot) error { return nil },
		Run: func(ctx context.Context) error {
			<-ctx.Done()
			return errors.New("server closed")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	registry.Start(ctx)
	cancel()
	registry.Wait()
	select {
	case err := <-registry.Errors():
		t.Fatalf("unexpected runner error = %v", err)
	default:
	}
}
