package plugin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/go-telegram/bot"
)

type RegisterCallback func(*bot.Bot) error
type RunCallback func(context.Context) error

type Callbacks struct {
	Register RegisterCallback
	Run      RunCallback
}

type registryEntry struct {
	name      string
	callbacks Callbacks
}

type Registry struct {
	entries []registryEntry
	wg      sync.WaitGroup
	errors  chan error
}

func NewRegistry() *Registry {
	return &Registry{errors: make(chan error, 1)}
}

func (r *Registry) Add(name string, callbacks Callbacks) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("plugin name must not be empty")
	}
	if callbacks.Register == nil {
		return fmt.Errorf("plugin %q has no register callback", name)
	}
	for _, entry := range r.entries {
		if entry.name == name {
			return fmt.Errorf("plugin %q is already registered", name)
		}
	}
	r.entries = append(r.entries, registryEntry{name: name, callbacks: callbacks})
	return nil
}

func (r *Registry) RegisterHandlers(b *bot.Bot) error {
	for _, entry := range r.entries {
		if err := entry.callbacks.Register(b); err != nil {
			return fmt.Errorf("register plugin %s: %w", entry.name, err)
		}
	}
	return nil
}

func (r *Registry) Start(ctx context.Context) {
	for _, entry := range r.entries {
		if entry.callbacks.Run == nil {
			continue
		}
		r.wg.Add(1)
		go func() {
			defer r.wg.Done()
			err := entry.callbacks.Run(ctx)
			if ctx.Err() != nil {
				return
			}
			if err == nil {
				err = errors.New("background service stopped unexpectedly")
			}
			select {
			case r.errors <- fmt.Errorf("run plugin %s: %w", entry.name, err):
			default:
			}
		}()
	}
}

func (r *Registry) Errors() <-chan error {
	return r.errors
}

func (r *Registry) Wait() {
	r.wg.Wait()
}

func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.entries))
	for _, entry := range r.entries {
		names = append(names, entry.name)
	}
	return names
}
