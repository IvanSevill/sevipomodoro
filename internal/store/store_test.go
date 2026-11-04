package store

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingUsesDefaults(t *testing.T) {
	s := NewAt(filepath.Join(t.TempDir(), "state.json"))
	state, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if state.Config.FocusMinutes != 25 {
		t.Fatalf("focus minutes = %d", state.Config.FocusMinutes)
	}
}

func TestMigrationAndCorruptStatePreserveOriginal(t *testing.T) {
	for _, tt := range []struct {
		name, raw string
		focused   int64
		theme     string
	}{
		{"legacy goal", `{"config":{"goal_minutes":120,"theme":"ocean"},"stats":{"focus_seconds":123,"completed_focus":7}}`, 123, "ocean"},
		{"invalid preferences", `{"version":2,"config":{"focus_minutes":0},"stats":{"focus_seconds":123}}`, 123, "ember"},
		{"negative stats", `{"version":2,"stats":{"focus_seconds":123,"completed_focus":-1}}`, 123, "ember"},
		{"malformed", `not-json`, 0, "ember"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			original := []byte(tt.raw)
			if err := os.WriteFile(path, original, 0600); err != nil {
				t.Fatal(err)
			}
			s := NewAt(path)
			state, err := s.Load()
			if err == nil {
				t.Fatal("expected migration/recovery warning")
			}
			if state.Config.TotalMinutes != 0 || state.Stats.FocusSeconds != tt.focused || state.Config.Theme != tt.theme {
				t.Fatalf("migration: %+v", state)
			}
			if err := s.Save(state); err != nil {
				t.Fatal(err)
			}
			backups, err := filepath.Glob(filepath.Join(filepath.Dir(path), "state-backup-*.json"))
			if err != nil || len(backups) != 1 {
				t.Fatalf("backups: %v, %v", backups, err)
			}
			data, err := os.ReadFile(backups[0])
			if err != nil || !bytes.Equal(data, original) {
				t.Fatal("original bytes not preserved")
			}
			loaded, err := NewAt(path).Load()
			if err != nil || loaded != state {
				t.Fatalf("v2 reload: %+v, %v", loaded, err)
			}
			if err := s.Save(state); err != nil {
				t.Fatal(err)
			}
			backups, _ = filepath.Glob(filepath.Join(filepath.Dir(path), "state-backup-*.json"))
			if len(backups) != 1 {
				t.Fatal("duplicate backup on ordinary save")
			}
		})
	}
}

func TestFutureVersionNeverOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	data := []byte(`{"version":99}`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	s := NewAt(path)
	state, err := s.Load()
	if err == nil {
		t.Fatal("missing future version warning")
	}
	if s.Save(state) == nil {
		t.Fatal("overwrote future state")
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal("future state changed")
	}
}

func TestFutureSchemaWithChangedFieldTypesNeverOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	data := []byte(`{"version":99,"config":{"focus_minutes":"new-format"}}`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	s := NewAt(path)
	state, err := s.Load()
	if err == nil || s.Save(state) == nil {
		t.Fatal("future schema was writable")
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal("future schema changed")
	}
}

func TestUnreadableStateNeverReplaced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	s := NewAt(path)
	state, err := s.Load()
	if err == nil {
		t.Fatal("expected read error")
	}
	// Make replacement possible after loading; a failed read must still block it.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(state); err == nil {
		t.Fatal("saved after unreadable state")
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "state.json")
	s := NewAt(path)
	want := DefaultState()
	want.Config.Theme = "ocean"
	want.Stats.CompletedFocus = 7

	if err := s.Save(want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got != want {
		t.Fatalf("state = %+v, want %+v", got, want)
	}
}

func TestLoadCorruptUsesDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewAt(path)
	state, err := s.Load()
	if err == nil {
		t.Fatal("Load() error = nil, want corruption warning")
	}
	if state != DefaultState() {
		t.Fatalf("state = %+v, want defaults", state)
	}
}
