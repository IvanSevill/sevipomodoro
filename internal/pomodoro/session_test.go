package pomodoro

import (
	"testing"
	"time"
)

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		valid  bool
	}{
		{name: "defaults", mutate: func(*Config) {}, valid: true},
		{name: "zero focus", mutate: func(c *Config) { c.FocusMinutes = 0 }},
		{name: "zero interval", mutate: func(c *Config) { c.SessionsBeforeLong = 0 }},
		{name: "unknown theme", mutate: func(c *Config) { c.Theme = "missing" }},
		{name: "maximum total", mutate: func(c *Config) { c.TotalMinutes = MaxTotalMinutes }, valid: true},
		{name: "negative total", mutate: func(c *Config) { c.TotalMinutes = -1 }},
		{name: "oversized total", mutate: func(c *Config) { c.TotalMinutes = MaxTotalMinutes + 1 }},
		{name: "oversized cadence", mutate: func(c *Config) { c.SessionsBeforeLong = MaxSessionsBeforeLong + 1 }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig()
			tt.mutate(&config)
			if got := config.Validate() == nil; got != tt.valid {
				t.Fatalf("Validate() success = %v, want %v", got, tt.valid)
			}
		})
	}
}

func TestSessionUsesDeadlineAndTransitions(t *testing.T) {
	config := DefaultConfig()
	config.FocusMinutes = 1
	config.ShortBreakMinutes = 1
	session := NewSession(config, Stats{})
	start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	session.Toggle(start)
	if got := session.Remaining(start.Add(17 * time.Second)); got != 43*time.Second {
		t.Fatalf("remaining = %v, want 43s", got)
	}
	if transitions := session.Tick(start.Add(time.Minute)); transitions != 1 {
		t.Fatalf("transitions = %d, want 1", transitions)
	}
	if session.Phase != ShortBreak || !session.Running {
		t.Fatalf("phase = %v, running = %v", session.Phase, session.Running)
	}
	if session.Stats.CompletedFocus != 1 || session.Stats.FocusSeconds != 60 {
		t.Fatalf("stats = %+v", session.Stats)
	}
}

func TestSessionLongBreakCycle(t *testing.T) {
	config := DefaultConfig()
	config.SessionsBeforeLong = 2
	session := NewSession(config, Stats{CompletedFocus: 1})
	now := time.Now()
	session.Toggle(now)
	session.Tick(now.Add(config.Duration(Focus)))

	if session.Phase != ShortBreak {
		t.Fatalf("phase = %v, lifetime stats must not affect new cadence", session.Phase)
	}
}

func TestTotalClipsEveryPhase(t *testing.T) {
	for _, tt := range []struct {
		name               string
		focus, rest, total int
		phase              Phase
		completed          int
		focused            time.Duration
	}{
		{"focus", 5, 2, 3, Focus, 0, 3 * time.Minute},
		{"regular break", 2, 5, 4, ShortBreak, 1, 2 * time.Minute},
		{"long break", 1, 1, 5, LongBreak, 2, 2 * time.Minute},
		{"two hours 35 plus 5", 35, 5, 120, ShortBreak, 3, 105 * time.Minute},
		{"exact focus boundary", 5, 2, 5, Focus, 1, 5 * time.Minute},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := DefaultConfig()
			c.FocusMinutes = tt.focus
			c.ShortBreakMinutes = tt.rest
			c.TotalMinutes = tt.total
			c.SessionsBeforeLong = 1
			if tt.phase == LongBreak {
				c.SessionsBeforeLong = 2
				c.LongBreakMinutes = 10
			}
			s := NewSession(c, Stats{})
			start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			s.Toggle(start)
			// Step exactly to the beginning of the final phase, then its boundary.
			now := start
			for !s.Final(now) {
				now = now.Add(s.Remaining(now) + s.FeedbackRemaining(now))
				s.Tick(now)
			}
			if s.Phase != tt.phase {
				t.Fatalf("final phase = %v", s.Phase)
			}
			if s.Remaining(now) != s.TotalRemaining(now) {
				t.Fatal("final phase not clipped")
			}
			end := now.Add(s.Remaining(now) + s.FeedbackRemaining(now))
			s.Tick(end.Add(-time.Nanosecond))
			if s.Ended {
				t.Fatal("ended early")
			}
			if n := s.Tick(end); n != 0 {
				t.Fatalf("completion emitted %d transitions", n)
			}
			if !s.Ended || s.Running || s.feedback != 0 || s.active != time.Duration(tt.total)*time.Minute {
				t.Fatalf("completion: %+v", s)
			}
			if s.Stats.CompletedFocus != tt.completed || s.Stats.FocusSeconds != int64(tt.focused/time.Second) {
				t.Fatalf("stats: %+v", s.Stats)
			}
			before := *s
			s.Tick(end.Add(24 * time.Hour))
			if *s != before {
				t.Fatal("ended timer changed")
			}
		})
	}
}

func TestCadenceAndFeedback(t *testing.T) {
	for _, cadence := range []int{1, 2, 3} {
		t.Run(string(rune('0'+cadence)), func(t *testing.T) {
			c := DefaultConfig()
			c.FocusMinutes = 1
			c.ShortBreakMinutes = 1
			c.LongBreakMinutes = 2
			c.SessionsBeforeLong = cadence
			s := NewSession(c, Stats{CompletedFocus: 71})
			now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			s.Toggle(now)
			for i := 1; i <= 6; i++ {
				now = now.Add(time.Minute)
				s.Tick(now)
				want := ShortBreak
				if cadence > 1 && i%cadence == 0 {
					want = LongBreak
				}
				if s.Phase != want || s.feedback != FeedbackDuration {
					t.Fatalf("block %d: %+v", i, s)
				}
				active := s.active
				now = now.Add(FeedbackDuration)
				s.Tick(now)
				if s.active != active || s.phaseElapsed != 0 {
					t.Fatal("feedback charged active budget")
				}
				now = now.Add(c.Duration(want))
				s.Tick(now)
				if s.Phase != Focus || s.feedback != FeedbackDuration {
					t.Fatal("missing focus feedback")
				}
				now = now.Add(FeedbackDuration)
				s.Tick(now)
			}
		})
	}
}

func TestDelayedTicksMatchFineTicks(t *testing.T) {
	c := DefaultConfig()
	c.FocusMinutes = 1
	c.ShortBreakMinutes = 1
	c.LongBreakMinutes = 2
	c.TotalMinutes = 13
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	a, b := NewSession(c, Stats{}), NewSession(c, Stats{})
	a.Toggle(start)
	b.Toggle(start)
	end := start.Add(12*time.Minute + 1723*time.Millisecond)
	a.Tick(end)
	for now := start; now.Before(end); now = now.Add(137 * time.Millisecond) {
		b.Tick(now)
	}
	b.Tick(end)
	if *a != *b {
		t.Fatalf("delayed %+v\nfine %+v", a, b)
	}
	before := *a
	a.Tick(end)
	a.Tick(start)
	if *a != before {
		t.Fatal("duplicate/stale tick consumed time")
	}
}

func TestPauseFreezesActiveAndFeedback(t *testing.T) {
	for _, pauseAt := range []time.Duration{20 * time.Second, 61 * time.Second} {
		t.Run(pauseAt.String(), func(t *testing.T) {
			c := DefaultConfig()
			c.FocusMinutes = 1
			c.TotalMinutes = 10
			s := NewSession(c, Stats{})
			start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			s.Toggle(start)
			now := start.Add(pauseAt)
			s.Toggle(now)
			phase, active, feedback := s.phaseElapsed, s.active, s.feedback
			now = now.Add(time.Hour)
			s.Tick(now)
			if s.phaseElapsed != phase || s.active != active || s.feedback != feedback {
				t.Fatal("pause consumed time")
			}
			s.Toggle(now)
			s.Tick(now.Add(feedback + time.Second))
			if s.active != active+time.Second || s.phaseElapsed != phase+time.Second {
				t.Fatal("resume charged feedback or pause")
			}
		})
	}
}

func TestSafetyWallClock(t *testing.T) {
	for _, mode := range []string{"running", "paused", "feedback", "settings applied", "finite"} {
		t.Run(mode, func(t *testing.T) {
			c := DefaultConfig()
			if mode == "finite" {
				c.TotalMinutes = 600
			}
			s := NewSession(c, Stats{})
			start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			s.Toggle(start)
			if mode == "paused" {
				s.Toggle(start.Add(time.Second))
			}
			if mode == "feedback" {
				s.Skip(start.Add(time.Second))
				s.Toggle(start.Add(2 * time.Second))
			}
			if mode == "settings applied" {
				c.FocusMinutes = 30
				s.ApplyConfig(c, start.Add(3*time.Hour))
			}
			s.Tick(start.Add(SafetyDuration - time.Nanosecond))
			if s.Ended {
				t.Fatal("early safety")
			}
			s.Tick(start.Add(SafetyDuration))
			if mode == "finite" {
				if s.Ended {
					t.Fatal("safety applied to explicit budget")
				}
				return
			}
			if !s.Ended || s.Running || s.EndReason != "Session ended — safety limit reached" {
				t.Fatalf("safety: %+v", s)
			}
		})
	}
}

func TestApplyPreservesProgress(t *testing.T) {
	for _, tt := range []struct {
		name         string
		focus, total int
		phase        Phase
		ended        bool
		remaining    time.Duration
	}{
		{"larger phase", 30, 60, Focus, false, 20 * time.Minute},
		{"smaller phase", 15, 60, Focus, false, 5 * time.Minute},
		{"exhausted phase", 5, 60, ShortBreak, false, 5 * time.Minute},
		{"exhausted total", 30, 5, Focus, true, 0},
		{"equal total wins", 5, 10, Focus, true, 0},
		{"larger total", 25, 120, Focus, false, 15 * time.Minute},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := DefaultConfig()
			c.TotalMinutes = 60
			s := NewSession(c, Stats{})
			start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			s.Toggle(start)
			now := start.Add(10 * time.Minute)
			c.FocusMinutes = tt.focus
			c.TotalMinutes = tt.total
			s.ApplyConfig(c, now)
			if s.active != 10*time.Minute || s.Phase != tt.phase || s.Ended != tt.ended || s.Remaining(now) != tt.remaining || s.Running == tt.ended {
				t.Fatalf("apply: %+v", s)
			}
			if s.Phase == Focus && s.phaseElapsed != 10*time.Minute {
				t.Fatal("lost phase progress")
			}
			if s.Phase == ShortBreak && (s.feedback != FeedbackDuration || s.phaseElapsed != 0) {
				t.Fatal("retroactive time charged to break")
			}
			if s.Stats.FocusSeconds != 600 {
				t.Fatal("focus double-counted")
			}
		})
	}
}

func TestApplyDuringRestAndPause(t *testing.T) {
	c := DefaultConfig()
	c.FocusMinutes = 1
	c.ShortBreakMinutes = 5
	c.TotalMinutes = 20
	s := NewSession(c, Stats{})
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.Toggle(start)
	now := start.Add(3*time.Minute + FeedbackDuration)
	s.Toggle(now)
	c.ShortBreakMinutes = 3
	s.ApplyConfig(c, now.Add(time.Hour))
	if s.Running || s.phaseElapsed != 2*time.Minute || s.Remaining(now.Add(time.Hour)) != time.Minute || s.active != 3*time.Minute {
		t.Fatalf("rest apply: %+v", s)
	}
	c.ShortBreakMinutes = 1
	s.ApplyConfig(c, now.Add(time.Hour))
	if s.Running || s.Phase != Focus || s.feedback != FeedbackDuration || s.active != 3*time.Minute {
		t.Fatalf("exhausted rest: %+v", s)
	}
}

func TestFiniteCadenceUsesActualSchedule(t *testing.T) {
	for _, total := range []int{1, 5, 60, 120, 1440} {
		for _, cadence := range []int{1, 2, 3, 50, MaxSessionsBeforeLong} {
			c := DefaultConfig()
			c.TotalMinutes = total
			c.SessionsBeforeLong = cadence
			c = c.NormalizeCadence()
			if c.SessionsBeforeLong > c.PlannedFocus() {
				t.Fatalf("cadence exceeds planned blocks: %+v", c)
			}
			if c.NormalizeCadence() != c {
				t.Fatal("normalization not stable")
			}
		}
	}
	c := DefaultConfig()
	c.SessionsBeforeLong = 999
	if c.NormalizeCadence() != c {
		t.Fatal("open-ended invented cycle cap")
	}
}

func TestCadenceBoundIndependentOfLongBreakLength(t *testing.T) {
	for _, tt := range []struct {
		name                            string
		total, focus, short, long, want int
	}{
		{"one clipped focus", 10, 25, 5, 15, 1},
		{"ends in first break", 30, 25, 5, 15, 1},
		{"clipped second focus", 31, 25, 5, 15, 2},
		{"longer replacement", 120, 25, 5, 120, 4},
		{"shorter replacement", 120, 25, 5, 1, 4},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := DefaultConfig()
			c.TotalMinutes, c.FocusMinutes = tt.total, tt.focus
			c.ShortBreakMinutes, c.LongBreakMinutes = tt.short, tt.long
			c.SessionsBeforeLong = MaxSessionsBeforeLong
			c = c.NormalizeCadence()
			if c.SessionsBeforeLong != tt.want {
				t.Fatalf("cadence = %d, want %d", c.SessionsBeforeLong, tt.want)
			}
		})
	}
}

func TestApplyNeverRestartsAndSafetyOriginSurvivesModeChanges(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := DefaultConfig()
	s := NewSession(c, Stats{})
	s.ApplyConfig(c, start)
	if s.Running || s.started {
		t.Fatal("apply started idle session")
	}
	s.Toggle(start)
	s.Toggle(start.Add(time.Minute))
	c.TotalMinutes = 600
	s.ApplyConfig(c, start.Add(3*time.Hour))
	s.Tick(start.Add(5 * time.Hour))
	if s.Ended || s.Running {
		t.Fatal("finite paused session should survive safety")
	}
	c.TotalMinutes = 0
	s.ApplyConfig(c, start.Add(5*time.Hour))
	if !s.Ended || s.EndReason != "Session ended — safety limit reached" {
		t.Fatal("lost safety origin")
	}
	c.TotalMinutes = 600
	s.ApplyConfig(c, start.Add(6*time.Hour))
	if !s.Ended || s.Running {
		t.Fatal("apply restarted ended session")
	}
}

func TestPauseDoesNotDrift(t *testing.T) {
	config := DefaultConfig()
	session := NewSession(config, Stats{})
	start := time.Now()
	session.Toggle(start)
	session.Toggle(start.Add(10 * time.Second))

	if got := session.Remaining(start.Add(time.Hour)); got != config.Duration(Focus)-10*time.Second {
		t.Fatalf("remaining while paused = %v", got)
	}
}

func TestSkipCountsPartialFocus(t *testing.T) {
	config := DefaultConfig()
	session := NewSession(config, Stats{})
	start := time.Now()
	session.Toggle(start)
	session.Skip(start.Add(90 * time.Second))

	if session.Stats.FocusSeconds != 90 || session.Stats.CompletedFocus != 0 {
		t.Fatalf("stats = %+v", session.Stats)
	}
}

func TestSkipAfterLongBreakStartsShortBreak(t *testing.T) {
	config := DefaultConfig()
	session := NewSession(config, Stats{CompletedFocus: config.SessionsBeforeLong})
	session.Phase = LongBreak
	session.Reset(time.Now())
	session.Skip(time.Now())
	session.Skip(time.Now())

	if session.Phase != ShortBreak {
		t.Fatalf("phase = %v, want short break", session.Phase)
	}
}

func TestCheckpointPersistsPartialFocus(t *testing.T) {
	config := DefaultConfig()
	session := NewSession(config, Stats{})
	start := time.Now()
	session.Toggle(start)
	session.Checkpoint(start.Add(45 * time.Second))

	if session.Stats.FocusSeconds != 45 || session.Running {
		t.Fatalf("stats = %+v, running = %v", session.Stats, session.Running)
	}
}
