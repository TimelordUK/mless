# mless

A modern, log-aware pager for the terminal. Like `less`, but with superpowers for log files: level filtering, time navigation, slicing with a revertable stack, split views, follow mode, and consolidated multi-file tailing.

## Features

- **Log level awareness** — auto-detects and color-codes `TRACE`, `DEBUG`, `INFO`, `WARN`, `ERROR`, `FATAL` (configurable aliases).
- **Level filtering** — toggle individual levels or show "level and above".
- **Live text filtering** — fzf-style narrowing as you type.
- **Search** — `/pattern` with `n`/`N` to jump between matches.
- **Time navigation** — jump to a timestamp (`14:30`, `14:30:00`, full date), works across logs that span midnight.
- **Marks** — bookmark up to 26 lines (`a`-`z`), survive filter changes, navigable with `]'` / `['`.
- **Slicing with a stack** — drill into a sub-range (lines, marks, time, current-to-end), then drill again, then `R` to pop back up the stack.
- **Split views** — vertical or horizontal, each pane has independent filters / search / marks.
- **Consolidated mode** (`-C`) — merge-tail multiple log files into a single time-ordered view (think `multitail`).
- **Follow mode** — `tail -f` style auto-scroll for growing files.
- **Yank to clipboard** — vim-style `yy`, `Nyy`, `y'a` (yank to mark), and a full visual mode. Works on macOS, Linux/X11/Wayland, Windows, and WSL (uses `clip.exe`).
- **Horizontal scrolling & wrap** — handle long lines without losing context.
- **Pipe support** — `kubectl logs ... | mless`, `grep err app.log | mless`.
- **Syntax highlighting** — when opened on a source file (Chroma-based), mless switches from log-level colouring to language syntax.
- **Vim-style count prefixes** — `5j`, `10yy`, `25k` all work.

## Installation

### From source

```bash
go install github.com/TimelordUK/mless/cmd/mless@latest
```

### Pre-built binaries

Download from the [releases page](https://github.com/TimelordUK/mless/releases) — Linux, macOS, and Windows, amd64 + arm64. Extract and put `mless` on your `PATH`.

### Chocolatey (Windows)

```powershell
choco install mless
```

*(Coming soon — see roadmap.)*

## Usage

```bash
# View a log file
mless application.log

# Open two files side-by-side (auto-split)
mless service-a.log service-b.log

# Consolidate multiple files into one time-ordered tail (-C)
mless -C kubelet.log api-server.log scheduler.log

# Initial slice on open
mless -S 1000-5000 application.log
mless -S 13:00-14:00 application.log

# Jump to a time on open
mless -t 14:30 application.log

# Cache a remote / network file locally for snappier navigation
mless -c /mnt/network/app.log

# Pipe from other commands
grep ERROR app.log | mless
kubectl logs -f deployment/myapp | mless
docker logs container | mless

# Print version
mless -v
```

`mless [-c] [-C] [-S range] [-t time] [file...]`

In normal mode up to 2 files open as a split. With `-C` there's no limit — they're merged into a single tailed view.

## Quick reference

| Key | Action |
|-----|--------|
| `j` / `k` | Scroll line by line (count prefix works: `5j`) |
| `f` / `b`, `space` | Page down / up |
| `ctrl+d` / `ctrl+u` | Half page |
| `g` / `G` | Top / bottom |
| `:N` | Go to line N |
| `ctrl+t` | Go to time |
| `/pattern` | Search |
| `n` / `N` | Next / previous match |
| `?pattern` | Live filter (fzf-style) |
| `0` | Clear all filters (preserves position) |
| `esc` | Clear search / filter / follow |
| `h` | Help screen |
| `ctrl+g` | File info |
| `q` / `ctrl+c` | Quit |

## Log level filtering

| Key | Action |
|-----|--------|
| `t` `d` `i` `w` `e` | Toggle TRACE / DEBUG / INFO / WARN / ERROR |
| `alt+f` | Toggle FATAL |
| `T` `D` `I` `W` `E` | Show level **and above** (cumulative) |
| `0` | Clear all level filters |

Recognised level keywords are configurable — see [Configuration](#configuration).

## Marks

Bookmark lines and jump between them. Marks are stored against the original line number, so they remain accurate across filter changes and slices.

| Key | Action |
|-----|--------|
| `m{a-z}` | Set mark at current line |
| `'{a-z}` | Jump to mark |
| `]'` / `['` | Next / previous mark |
| `M` | Clear all marks |

## Slicing (with a stack)

A *slice* extracts a portion of the file into a temp cache and views it as if it were the whole file. Slices are **stacked** — you can slice a slice, and `R` pops back up.

| Key | Action |
|-----|--------|
| `S` | Slice range (prompt) |
| `ctrl+s` | Slice from current line to end |
| `R` | Pop the slice stack (or resync cache if `-c` was used) |

The status bar shows `[slice:start-end]` for a single slice and `[slice×N:start-end]` when the stack has multiple levels.

Range syntax accepted by `S` and `-S`:

| Range | Meaning |
|-------|---------|
| `100-500` | Lines 100–500 |
| `100-$` | Line 100 to end |
| `.-$` | Current line to end |
| `$-1000-$` | Last 1000 lines |
| `'a-'b` | Between marks `a` and `b` |
| `13:00-14:00` | Time range |
| `13:00-.` | From 13:00 to current |

## Yank (copy to system clipboard)

Vim-style. Counts and marks both work.

| Key | Action |
|-----|--------|
| `yy` / `Y` | Yank current line |
| `5yy` | Yank 5 lines starting from current |
| `y'a` | Yank from current line to mark `a` |
| `v` | Enter visual selection mode |

In **visual mode**:

| Key | Action |
|-----|--------|
| `j` / `k` | Extend selection |
| `g` / `G` | Extend to top / bottom |
| `f` / `b` | Page down / up |
| `y` | Yank selection |
| `v` / `esc` | Cancel |

The status bar shows `-- VISUAL -- N lines selected (L# - L#)`. Clipboard backends: `pbcopy` (macOS), `clip.exe` (WSL), `xclip` / `xsel` / `wl-copy` (Linux), `clip` (Windows).

## Long lines

| Key | Action |
|-----|--------|
| `<` / `>` (or `←` / `→`) | Scroll horizontally 10 cols |
| `^` | Reset horizontal scroll |
| `Z` | Toggle line wrap |
| `l` | Show line numbers |

## Split views

Each pane keeps its own filters, marks, search, viewport — the only thing they share is the underlying file source.

| Key | Action |
|-----|--------|
| `ctrl+w v` | Vertical split (side-by-side) |
| `ctrl+w s` | Horizontal split (stacked) |
| `ctrl+w w` / `tab` | Switch active pane |
| `ctrl+w q` | Close current pane |
| `ctrl+o` | Toggle split orientation |
| `H` / `L` | Resize splitter (5% steps) |
| `=` | Reset to 50/50 |

The active pane is indicated by a bold separator (`┃` / `━`).

## Consolidated mode (`-C`)

For watching N services at once. mless primes with the last 100 lines from each file, then tails for new content, writing to a single backing file in time order. The status bar shows `[consolidated: N files]`.

```bash
mless -C api.log worker.log scheduler.log
```

Lines preserve their original ordering by timestamp where present. Follow mode is enabled by default in `-C`.

## Time navigation

| Key | Action |
|-----|--------|
| `ctrl+t` | Prompt for time |

Accepted formats: `15:04`, `15:04:05`, `2006-01-02 15:04`, `2006-01-02 15:04:05`, `2006-01-02T15:04:05`. Time-only inputs use the date of the first log line. Logs that span midnight are handled automatically.

After jumping you'll see a status message like `Target 14:30:00 -> 2024-05-12 14:29:58.341` showing where you actually landed.

## Follow mode

| Key | Action |
|-----|--------|
| `F` | Toggle follow (tail -f) |
| `esc` | Stop following |

Polls every 500ms; auto-scrolls to bottom when new lines arrive.

## File info (`ctrl+g`)

Shows source path, total lines, current position, active filters, slice info, cache path, and marks.

## Configuration

mless reads `~/.config/mless/config.toml` if present. Example:

```toml
[display]
show_line_numbers = true

[log_levels]
trace = ["TRACE", "TRC"]
debug = ["DEBUG", "DBG"]
info  = ["INFO", "INF"]
warn  = ["WARN", "WRN", "WARNING"]
error = ["ERROR", "ERR"]
fatal = ["FATAL", "FTL", "CRITICAL"]

[colors]
trace = "240"
debug = "244"
info  = "white"
warn  = "yellow"
error = "red"
fatal = "red"
```

See `config.example.toml` for the full set of options.

## Examples

### Triage an incident

```bash
mless app.log
E              # show errors and above
/timeout       # find the relevant ones
ma             # mark the first interesting line
G              # jump to end
mb             # mark the last
S'a-'b         # slice down to that window
0              # drop the level filter to see context inside the slice
ctrl+w v       # split: errors on left
tab; 0         # full context on right
```

### Watch a fleet

```bash
mless -C api.log worker.log scheduler.log
?5xx           # live-filter to anything mentioning 5xx
F              # follow
```

### Compare time windows

```bash
mless app.log
ctrl+w v
S09:00-12:00   # left pane: morning
tab
S14:00-17:00   # right pane: afternoon
```

## Building from source

```bash
make build       # builds ./mless with version info baked in
make install     # installs to $GOPATH/bin
make patch       # tag a new patch release (v0.x.Y -> v0.x.Y+1)
make minor       # tag a new minor release
```

Release artifacts are produced by GoReleaser via the `release` GitHub Action when a `v*` tag is pushed (or via manual `workflow_dispatch`).

## Roadmap

See [ROADMAP.md](ROADMAP.md). Highlights still on the list:

- Time-synced split scrolling
- Resizable splits with mouse
- JSONL-aware view (auto-detect, key projection, structured filtering)
- Chocolatey + Homebrew distribution

## License

MIT — see [LICENSE](LICENSE).

## Contributing

Issues and PRs welcome at <https://github.com/TimelordUK/mless>.
