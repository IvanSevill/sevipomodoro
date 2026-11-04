package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ivansevill/sevipomodoro/internal/pomodoro"
	"github.com/ivansevill/sevipomodoro/internal/store"
)

type fakeSaver struct {
	state store.State
	calls int
}

func key(m Model, value string) Model {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(value)}
	switch value {
	case "enter":
		msg.Type = tea.KeyEnter
	case "esc":
		msg.Type = tea.KeyEsc
	case " ":
		msg.Type = tea.KeySpace
	}
	updated, _ := m.Update(msg)
	return updated.(Model)
}

func TestRunningSettingsDraftPreviewDiscardAndApply(t *testing.T) {
	for _, action := range []string{"discard", "apply"} {
		t.Run(action, func(t *testing.T) {
			now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			start := now
			saver := &fakeSaver{}
			m := NewModel(store.DefaultState(), saver, "")
			m.now = func() time.Time { return now }
			m.width, m.height = 80, 24
			m = key(m, " ")
			now = start.Add(time.Minute)
			m = key(m, "t")
			if !m.session.Running {
				t.Fatal("opening settings paused")
			}
			m = key(m, "l")
			if m.session.Config.FocusMinutes != 25 || m.draft.FocusMinutes != 26 {
				t.Fatal("timing edits not draft")
			}
			m.settingRow = 6
			m = key(m, "l")
			m.settingRow = 5
			m = key(m, "l")
			if !m.session.Running || m.session.Config.Theme != "ember" || m.draft.Theme != "ocean" || !m.bellEnabled() {
				t.Fatal("preview changed timing or failed")
			}
			now = start.Add(2 * time.Minute)
			updated, _ := m.Update(tickMsg(now))
			m = updated.(Model)
			if m.session.ActiveElapsed(now) != 2*time.Minute {
				t.Fatal("settings browsing paused")
			}
			now = start.Add(150 * time.Second)
			if action == "discard" {
				m = key(m, "esc")
				if m.session.Config != pomodoro.DefaultConfig() || m.bellEnabled() || saver.calls != 0 {
					t.Fatal("discard leaked draft")
				}
			} else {
				m = key(m, "enter")
				if m.session.Config.FocusMinutes != 26 || m.session.Config.Theme != "ocean" || !m.session.Config.Bell {
					t.Fatal("apply lost draft")
				}
				if saver.state.Stats.FocusSeconds != 150 || saver.calls != 1 {
					t.Fatalf("save used stale stats: %+v", saver)
				}
			}
			if !m.session.Running || m.session.ActiveElapsed(now) != 150*time.Second || m.session.Elapsed(now) != 150*time.Second {
				t.Fatal("settings reset progress")
			}
		})
	}
}

func TestSettingsSafetyAndFeedbackRemainVisible(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	start := now
	state := store.DefaultState()
	state.Config.FocusMinutes = 1
	m := NewModel(state, nil, "")
	m.now = func() time.Time { return now }
	m.width, m.height = 80, 24
	m = key(m, " ")
	m = key(m, "t")
	now = start.Add(time.Minute)
	updated, _ := m.Update(tickMsg(now))
	m = updated.(Model)
	if !strings.Contains(m.View(), "Time for a break") || m.session.ActiveElapsed(now) != time.Minute {
		t.Fatal("feedback missing while settings open")
	}
	m = key(m, "esc")
	m = key(m, " ")
	m = key(m, "t")
	now = start.Add(pomodoro.SafetyDuration)
	updated, _ = m.Update(tickMsg(now))
	m = updated.(Model)
	if !m.session.Ended || !strings.Contains(m.View(), "Session ended — safety limit reached") {
		t.Fatal("safety missed during paused settings")
	}
}

func TestKeyAtDeadlinePersistsAndSchedulesBell(t *testing.T) {
	for _, total := range []int{0, 1} {
		t.Run(map[bool]string{true: "completion", false: "transition"}[total > 0], func(t *testing.T) {
			now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			state := store.DefaultState()
			state.Config.FocusMinutes, state.Config.TotalMinutes, state.Config.Bell = 1, total, true
			saver := &fakeSaver{}
			m := NewModel(state, saver, "")
			m.now = func() time.Time { return now }
			m = key(m, " ")
			now = now.Add(time.Minute)
			updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
			m = updated.(Model)
			if saver.calls != 1 || saver.state.Stats.CompletedFocus != 1 || saver.state.Stats.FocusSeconds != 60 {
				t.Fatalf("deadline key did not persist: %+v", saver)
			}
			if total == 0 && (cmd == nil || m.session.Phase != pomodoro.ShortBreak) {
				t.Fatal("missing transition notification")
			}
			if total > 0 && (cmd != nil || !m.session.Ended) {
				t.Fatal("completion must not schedule next-phase bell")
			}
		})
	}
}

func TestDisabledLongBreakAndFiniteCadenceInput(t *testing.T) {
	m := NewModel(store.DefaultState(), nil, "")
	m = key(m, "t")
	m.draft.SessionsBeforeLong = 1
	m.settingRow = 2
	before := m.draft.LongBreakMinutes
	m = key(m, "l")
	if m.draft.LongBreakMinutes != before {
		t.Fatal("disabled long break edited")
	}
	m.width, m.height = 80, 24
	if !strings.Contains(m.View(), "Disabled") {
		t.Fatal("missing Disabled label")
	}
	m.draft.TotalMinutes = 30
	m.settingRow = 3
	m = key(m, "l")
	if m.draft.SessionsBeforeLong > m.draft.PlannedFocus() {
		t.Fatal("finite cadence exceeds schedule")
	}
	m.draft.TotalMinutes = 0
	m.draft.SessionsBeforeLong = 100
	m = key(m, "l")
	if m.draft.SessionsBeforeLong != 101 {
		t.Fatal("fabricated open-ended cycle cap")
	}
}

func (s *fakeSaver) Save(state store.State) error {
	s.state = state
	s.calls++
	return nil
}

func TestModelTimerControls(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	m := NewModel(store.DefaultState(), nil, "")
	m.now = func() time.Time { return now }

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(Model)
	if !m.session.Running {
		t.Fatal("session should be running after space")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = updated.(Model)
	if m.session.Running {
		t.Fatal("session should be stopped after reset")
	}
}

func TestSettingsAdjustAndPersist(t *testing.T) {
	saver := &fakeSaver{}
	m := NewModel(store.DefaultState(), saver, "")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if m.settings {
		t.Fatal("settings should close after save")
	}
	if m.session.Config.FocusMinutes != 26 {
		t.Fatalf("focus minutes = %d, want 26", m.session.Config.FocusMinutes)
	}
	if saver.calls != 1 {
		t.Fatalf("save calls = %d, want 1", saver.calls)
	}
}

func TestWindowSizeUpdatesLayout(t *testing.T) {
	m := NewModel(store.DefaultState(), nil, "")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 42, Height: 20})
	m = updated.(Model)
	if m.width != 42 || m.height != 20 {
		t.Fatalf("size = %dx%d", m.width, m.height)
	}
	if view := m.View(); view == "" {
		t.Fatal("View() returned empty output")
	}
}
