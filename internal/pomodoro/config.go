package pomodoro

import (
	"fmt"
	"time"
)

const (
	MinFocusMinutes       = 1
	MaxFocusMinutes       = 180
	MinBreakMinutes       = 1
	MaxBreakMinutes       = 120
	MinSessionsBeforeLong = 1
	MaxSessionsBeforeLong = 1_000_000 // Input bound, not a session limit.
	MaxTotalMinutes       = 24 * 60
)

type Config struct {
	FocusMinutes       int    `json:"focus_minutes"`
	ShortBreakMinutes  int    `json:"short_break_minutes"`
	LongBreakMinutes   int    `json:"long_break_minutes"`
	SessionsBeforeLong int    `json:"sessions_before_long"`
	TotalMinutes       int    `json:"total_minutes"`
	Bell               bool   `json:"bell"`
	Theme              string `json:"theme"`
}

func DefaultConfig() Config {
	return Config{
		FocusMinutes:       25,
		ShortBreakMinutes:  5,
		LongBreakMinutes:   15,
		SessionsBeforeLong: 4,
		Theme:              "ember",
	}
}

func (c Config) Validate() error {
	switch {
	case c.FocusMinutes < MinFocusMinutes || c.FocusMinutes > MaxFocusMinutes:
		return fmt.Errorf("focus duration must be %d-%d minutes", MinFocusMinutes, MaxFocusMinutes)
	case c.ShortBreakMinutes < MinBreakMinutes || c.ShortBreakMinutes > MaxBreakMinutes:
		return fmt.Errorf("short break must be %d-%d minutes", MinBreakMinutes, MaxBreakMinutes)
	case c.LongBreakMinutes < MinBreakMinutes || c.LongBreakMinutes > MaxBreakMinutes:
		return fmt.Errorf("long break must be %d-%d minutes", MinBreakMinutes, MaxBreakMinutes)
	case c.SessionsBeforeLong < MinSessionsBeforeLong || c.SessionsBeforeLong > MaxSessionsBeforeLong:
		return fmt.Errorf("long-break interval must be %d-%d sessions", MinSessionsBeforeLong, MaxSessionsBeforeLong)
	case c.TotalMinutes < 0 || c.TotalMinutes > MaxTotalMinutes:
		return fmt.Errorf("total duration must be 0-%d minutes", MaxTotalMinutes)
	case !ValidTheme(c.Theme):
		return fmt.Errorf("unknown theme %q", c.Theme)
	default:
		return nil
	}
}

func (c Config) Duration(phase Phase) time.Duration {
	minutes := c.FocusMinutes
	switch phase {
	case ShortBreak:
		minutes = c.ShortBreakMinutes
	case LongBreak:
		minutes = c.LongBreakMinutes
	}
	return time.Duration(minutes) * time.Minute
}

// PlannedFocus counts blocks that start, including a clipped last focus block.
// Feedback is outside the active budget; lifetime stats never affect cadence.
func (c Config) PlannedFocus() int {
	if c.TotalMinutes == 0 {
		return 0
	}
	left, count := c.TotalMinutes, 0
	for left > 0 {
		count++
		left -= max(1, c.FocusMinutes)
		if left <= 0 {
			break
		}
		rest := max(1, c.ShortBreakMinutes)
		if c.SessionsBeforeLong > 1 && count%c.SessionsBeforeLong == 0 {
			rest = max(1, c.LongBreakMinutes)
		}
		left -= rest
	}
	return count
}

func (c Config) NormalizeCadence() Config {
	if c.TotalMinutes > 0 {
		// Bound against the short-break plan, independently of cadence. The
		// first long break cannot affect whether its preceding focus starts.
		limit := 1 + (c.TotalMinutes-1)/(max(1, c.FocusMinutes)+max(1, c.ShortBreakMinutes))
		c.SessionsBeforeLong = min(c.SessionsBeforeLong, limit)
	}
	return c
}

func ValidTheme(theme string) bool {
	for _, name := range ThemeNames() {
		if theme == name {
			return true
		}
	}
	return false
}

func ThemeNames() []string {
	return []string{"ember", "ocean", "forest", "mono"}
}
