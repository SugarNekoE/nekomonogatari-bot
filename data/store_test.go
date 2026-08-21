package data

import (
	"context"
	"path/filepath"
	"testing"
)

type testRecord struct {
	ID    uint `gorm:"primaryKey"`
	Value string
}

func (testRecord) TableName() string {
	return "test-plugin_records"
}

func TestOpenAndMigrate(t *testing.T) {
	t.Parallel()

	store, err := Open(context.Background(), filepath.Join(t.TempDir(), "nested", "bot.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if err := store.DB().AutoMigrate(&testRecord{}); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}
	if err := store.DB().Create(&testRecord{Value: "ok"}).Error; err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	var got testRecord
	if err := store.DB().First(&got).Error; err != nil {
		t.Fatalf("First() error = %v", err)
	}
	if got.Value != "ok" {
		t.Fatalf("Value = %q", got.Value)
	}
}

func TestMemoryStoresAreIsolated(t *testing.T) {
	t.Parallel()

	first, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })
	second, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close() })

	if err := first.DB().AutoMigrate(&testRecord{}); err != nil {
		t.Fatal(err)
	}
	if second.DB().Migrator().HasTable(&testRecord{}) {
		t.Fatal("independent in-memory stores share tables")
	}
}

func TestForeignKeysEnabled(t *testing.T) {
	t.Parallel()

	store, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	var enabled int
	if err := store.DB().Raw("PRAGMA foreign_keys").Scan(&enabled).Error; err != nil {
		t.Fatal(err)
	}
	if enabled != 1 {
		t.Fatalf("foreign_keys = %d", enabled)
	}
}
