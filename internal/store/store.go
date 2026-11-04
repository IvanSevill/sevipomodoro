package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ivansevill/sevipomodoro/internal/pomodoro"
)

type State struct {
	Version int             `json:"version"`
	Config  pomodoro.Config `json:"config"`
	Stats   pomodoro.Stats  `json:"stats"`
}

func DefaultState() State {
	return State{Version: 2, Config: pomodoro.DefaultConfig()}
}

type FileStore struct {
	path     string
	original []byte
	readOnly bool
}

func New() (*FileStore, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("resolve config directory: %w", err)
	}
	return NewAt(filepath.Join(dir, "sevipomodoro", "state.json")), nil
}

func NewAt(path string) *FileStore {
	return &FileStore{path: path}
}

func (s *FileStore) Path() string {
	return s.path
}

func (s *FileStore) Load() (State, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return DefaultState(), nil
	}
	if err != nil {
		s.readOnly = true
		return DefaultState(), fmt.Errorf("read state: %w", err)
	}
	s.original = append([]byte(nil), data...)
	// Inspect the schema before decoding fields whose types may change later.
	var header struct {
		Version int `json:"version"`
	}
	if json.Unmarshal(data, &header) == nil && header.Version > 2 {
		s.readOnly = true
		return DefaultState(), fmt.Errorf("state is from a newer version; saving disabled")
	}
	state := DefaultState()
	state.Version = 0
	if err := json.Unmarshal(data, &state); err != nil {
		return DefaultState(), fmt.Errorf("decode state: %w", err)
	}
	if state.Version > 2 {
		s.readOnly = true
		return DefaultState(), fmt.Errorf("state is from a newer version; saving disabled")
	}
	var warning error
	if state.Version < 2 {
		state.Config.TotalMinutes = 0
		warning = fmt.Errorf("legacy preferences migrated; focus goal cleared — choose Total duration")
	}
	if err := state.Config.Validate(); err != nil {
		state.Config = pomodoro.DefaultConfig()
		warning = fmt.Errorf("invalid preferences replaced; valid statistics retained")
	}
	if state.Stats.CompletedFocus < 0 || state.Stats.FocusSeconds < 0 {
		state.Stats.CompletedFocus = max(0, state.Stats.CompletedFocus)
		state.Stats.FocusSeconds = max(0, state.Stats.FocusSeconds)
		warning = fmt.Errorf("invalid statistics repaired")
	}
	state.Config = state.Config.NormalizeCadence()
	state.Version = 2
	if warning != nil {
		return state, warning
	}
	s.original = nil
	return state, nil
}

func (s *FileStore) Save(state State) error {
	if s.readOnly {
		return fmt.Errorf("refusing to overwrite unreadable or newer state")
	}
	if err := state.Config.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	// Preserve exact legacy/corrupt bytes before the first replacement. Unique
	// backups never overwrite an earlier recovery file.
	if s.original != nil {
		backup, err := os.CreateTemp(filepath.Dir(s.path), "state-backup-*.json")
		if err != nil {
			return fmt.Errorf("back up original state: %w", err)
		}
		_, writeErr := backup.Write(s.original)
		closeErr := backup.Close()
		if writeErr != nil {
			return writeErr
		}
		if closeErr != nil {
			return closeErr
		}
		s.original = nil
	}
	state.Version = 2
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".state-*.json")
	if err != nil {
		return fmt.Errorf("create temporary state: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("protect temporary state: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temporary state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary state: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("replace state: %w", err)
	}
	return nil
}
