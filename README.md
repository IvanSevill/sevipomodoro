# Sevi Pomodoro

A focused, keyboard-first Pomodoro terminal application built with Go, Bubble Tea, and Lip Gloss.

```text
┏━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┓
┃                           FOCUS  /  PAUSED                           ┃
┃                              Open-ended                              ┃
┃                                                                      ┃
┃                       ┏━┓  ┏━╸       ┏━┓  ┏━┓                        ┃
┃                         ┃  ┃     ▪   ┃ ┃  ┃ ┃                        ┃
┃                       ┏━┛  ┗━┓       ┃ ┃  ┃ ┃                        ┃
┃                       ┃      ┃   ▪   ┃ ┃  ┃ ┃                        ┃
┃                       ┗━╸  ┗━┛       ┗━┛  ┗━┛                        ┃
┃                                                                      ┃
┃   ────────────────────────────────────────────────────────────────   ┃
┃                              0% elapsed                              ┃
┃                                                                      ┃
┃                    0 sessions  ·  0h 00m focused                     ┃
┃                                                                      ┃
┃                                                                      ┃
┃             space/enter start/pause · r reset · s/n skip             ┃
┃                 t/, settings · q quit · ctrl+c exit                  ┃
┃                                                                      ┃
┗━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┛

```

## Features

- Automatic focus, short-break, and long-break transitions
- Timestamp-based timing that does not accumulate tick drift
- Configurable focus/rest durations and optional long breaks
- Optional total active-time budget, including both focus and breaks
- Persistent statistics and preferences with safe fallback for missing or corrupt state
- Four built-in themes: Ember, Ocean, Forest, and Mono
- Responsive full-screen layout, focus halo, distinct break border, and optional terminal bell

Defaults are 25 minutes of focus, a 5-minute short break, a 15-minute long break, and a long break after every 4 completed focus sessions.

## Requirements

- Go 1.24 or newer
- A terminal with color support

## Install

```sh
go install github.com/ivansevill/sevipomodoro/cmd/sevipomodoro@latest
```

This places `sevipomodoro` in your Go bin directory.

## Run from source

Clone the repository, then from its directory:

```sh
make build
./bin/sevipomodoro
```

Run without installing:

```sh
go run ./cmd/sevipomodoro
```

Print the locally built version without starting the TUI:

```sh
./bin/sevipomodoro -version
```

## Controls

### Timer

| Key | Action |
| --- | --- |
| `Space` or `Enter` | Start or pause |
| `r` | Reset and pause the current phase, preserving spent total and safety start; after completion, prepare a new session |
| `s` or `n` | Skip to the next phase |
| `t` or `,` | Open settings |
| `q` or `Ctrl+C` | Quit |

### Settings

| Key | Action |
| --- | --- |
| `Up` / `Down`, `k` / `j`, `Tab` | Select a setting |
| `Left` / `Right`, `h` / `l`, `-` / `+` | Adjust the selected value |
| `Enter` | Apply and save, preserving elapsed progress and running/paused state |
| `Esc`, `q`, or `t` | Discard changes |

Total duration changes in 5-minute increments (0–1,440 minutes). Zero displays `Open-ended`. Focus is bounded to 1–180 minutes and breaks to 1–120 minutes. Only explicit pause controls pause a running timer. Reset and skip never refund spent total time or extend the safety deadline.

## Timing behavior

| Setting or action | Behavior |
| --- | --- |
| Total duration | Focus **plus breaks**, excluding explicit pauses and transition feedback. A 120-minute budget at 35-minute focus / 5-minute rest ends after the third rest. |
| Final session | The last focus **or break** is clipped to the remaining budget. The total deadline wins, with no completion transition. |
| Open-ended | No displayed cycle limit. An internal safety cutoff ends the session exactly four hours after starting, including pauses, settings, and feedback. The next UI update reports `Session ended — safety limit reached`. Explicit total durations have no four-hour cutoff. |
| Long break every | `Disabled` (stored cadence 1), or every N completed focus blocks: 2 = regular/long; 3 = regular/regular/long. Long breaks replace regular breaks. |
| Phase switches | Three seconds of `Time to focus` / `Time for a break`, with no active-budget consumption. Pausing also freezes this feedback. An optional terminal bell accompanies switches. |
| Settings | The timer keeps running. Timing changes are drafts; theme and bell preview live. Apply preserves elapsed active total and current-phase time. Discard restores previews without changing timer progress. |

Finite cadence is bounded by focus blocks that would start using only short breaks, including a clipped final focus: `1 + (total - 1) / (focus + short break)` with integer division in minutes. This is independent of the chosen cadence, so normalization never recursively recalculates its own bound. A one-block plan disables long breaks; a clipped final focus may mean the last planned long break is never reached. Even if a long break is shorter than a short break, the bound stays conservative. Duration edits recalculate the bound. Open-ended cadence has only an input bound (1–1,000,000), not a cycle cap. Disabled long-break duration cannot be edited. Cadence counts completed focus blocks in the current session, independently of lifetime statistics; skipping focus does not count as completion.

Applying a shorter duration can exhaust the current phase: it advances once, with feedback, and never charges past elapsed time to the next phase. If the new total is already spent, the session ends immediately. Applying larger values extends only the remainder. Paused sessions stay paused. Changing cadence affects future breaks; a break already in progress retains its phase. Switching to open-ended mode keeps the original start timestamp and ends immediately if four hours have already elapsed. An ended session stays ended until reset.

Delayed ticks are split at exact boundaries, including feedback, without drift or double-counting. A suspended terminal cannot display missed frames; catch-up accounts for those feedback intervals rather than restarting them on wake.

## Persistence

Configuration and statistics are stored in the operating system's user configuration directory:

- Linux: typically `~/.config/sevipomodoro/state.json`
- macOS: typically `~/Library/Application Support/sevipomodoro/state.json`
- Windows: typically `%AppData%\\sevipomodoro\\state.json`

Writes are atomic and use owner-only permissions where supported. Saves include current focus statistics without restarting the timer. Only preferences and lifetime statistics persist; running sessions do not resume after process exit. Partial focus contributes to statistics; a clipped or skipped focus does not count as a completed block.

State schema version 2 stores `total_minutes`. Legacy `goal_minutes` represented accumulated focus, so migration **clears it to Open-ended**, displays a warning, and keeps valid preferences/statistics. Choose a new Total duration explicitly. Legacy cadence 1 now means Disabled. Invalid preferences fall back to defaults while valid statistics survive; negative statistics are repaired independently. Before replacing legacy or corrupt state, its exact bytes are preserved in an owner-only `state-backup-*.json` file alongside the state. Unknown future schema versions are never overwritten. Malformed JSON starts with defaults and a warning.

## Development

```sh
make fmt
make test
make check
go test -race ./...
go vet ./...
go build ./...
```

The timer state machine lives in `internal/pomodoro` and accepts explicit timestamps, keeping transitions independently testable from Bubble Tea rendering.

Regenerate deterministic view snapshots with `go test ./internal/tui -run TestViewGolden -update`, inspect them, then rerun `go test ./...`. Domain and settings regression tests use injected timestamps, not sleeps. Filesystem tests use temporary directories; isolate `XDG_CONFIG_HOME` for manual or PTY runs.

## License

[MIT](LICENSE) © Ivan J. Sevillano
