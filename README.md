# mless

A modern, log-aware pager for the terminal. Like `less`, but with superpowers for log files.

## Features

- **Log Level Awareness** - Automatically detects and color-codes log levels (TRACE, DEBUG, INFO, WARN, ERROR, FATAL)
- **Level Filtering** - Toggle visibility of log levels or show "level and above"
- **Split Views** - Side-by-side or stacked panes, each with independent filters
- **Time Navigation** - Jump to specific timestamps with `ctrl+t`
- **Marks** - Bookmark lines and navigate between them
- **Slicing** - Extract portions of logs by line range, marks, or time
- **Live Filtering** - fzf-style text filtering
- **Pipe Support** - Use with pipes: `grep pattern file.log | mless`
- **Follow Mode** - Like `tail -f` for growing files

## Installation

```bash
go install github.com/TimelordUK/mless/cmd/mless@latest
```

## Usage

```bash
# View a log file
mless application.log

# With initial slice
mless -S 1000-5000 application.log

# Jump to specific time on start
mless -t 14:30 application.log

# Cache remote/network file locally
mless -c /mnt/network/app.log

# Pipe from other commands
grep "ERROR" app.log | mless
kubectl logs pod-name | mless
docker logs container | mless
```

## Quick Start

| Key | Action |
|-----|--------|
| `j/k` | Scroll up/down |
| `g/G` | Go to top/bottom |
| `/pattern` | Search |
| `?pattern` | Filter lines (live) |
| `i/w/e` | Toggle INFO/WARN/ERROR |
| `I/W/E` | Show level and above |
| `0` | Clear all filters |
| `h` | Show help |
| `q` | Quit |

## Split Views

Create split views to compare different portions or filter views of the same log:

| Key | Action |
|-----|--------|
| `ctrl+w v` | Vertical split (side-by-side) |
| `ctrl+w s` | Horizontal split (stacked) |
| `tab` | Switch between panes |
| `ctrl+w q` | Close current pane |

Each pane has **independent filters** - view ERRORs in one pane while seeing the full context in another!

## Log Level Filtering

Toggle individual levels or show cumulative levels:

| Key | Action |
|-----|--------|
| `t/d/i/w/e` | Toggle TRACE/DEBUG/INFO/WARN/ERROR |
| `alt+f` | Toggle FATAL |
| `T/D/I/W/E` | Show level and above |
| `0` | Clear all level filters |

## Time Navigation

Navigate logs by timestamp:

| Key | Action |
|-----|--------|
| `ctrl+t` | Go to time (enter HH:MM or HH:MM:SS) |

Works with common timestamp formats in your logs.

## Marks

Bookmark important lines for quick navigation:

| Key | Action |
|-----|--------|
| `ma` - `mz` | Set mark a-z at current line |
| `'a` - `'z` | Jump to mark |
| `]'` / `['` | Next/previous mark |
| `M` | Clear all marks |

## Slicing

Extract portions of logs to focus on specific sections:

| Key | Action |
|-----|--------|
| `S` | Slice by range |
| `ctrl+s` | Slice from current line to end |
| `R` | Revert to previous slice/full file |

Slice range examples:
- `S100-500` - Lines 100 to 500
- `S'a-'b` - Between marks a and b
- `S13:00-14:00` - Time range
- `S.-$` - Current line to end
- `S$-1000-$` - Last 1000 lines

## Other Features

| Key | Action |
|-----|--------|
| `F` | Toggle follow mode (tail -f) |
| `ctrl+g` | Show file info (path, lines, marks) |
| `n/N` | Next/previous search result |
| `:123` | Go to line 123 |
| `esc` | Clear search/filter/follow |

## Examples

### Debugging an Issue

```bash
# Open the log
mless app.log

# Filter to errors only
e

# Search for specific error
/connection refused

# Set marks at interesting points
ma  # mark start of issue
mb  # mark end of issue

# Slice to just that section
S'a-'b

# Split view: errors on left, full context on right
ctrl+w v
tab
0  # clear filter in right pane
```

### Comparing Time Periods

```bash
mless app.log

# Create vertical split
ctrl+w v

# Left pane: morning logs
S09:00-12:00

# Switch to right pane
tab

# Right pane: afternoon logs
S14:00-17:00

# Each pane can have different level filters!
```

### Pipeline Usage

```bash
# Filter before viewing
grep -i error app.log | mless

# Kubernetes logs
kubectl logs -f deployment/myapp | mless

# Docker with timestamps
docker logs --timestamps container | mless

# Multiple files
cat *.log | mless
```

## Configuration

mless looks for `~/.config/mless/config.yaml` for customization:

```yaml
display:
  show_line_numbers: true

log_levels:
  trace: ["TRACE", "TRC"]
  debug: ["DEBUG", "DBG"]
  info: ["INFO", "INF"]
  warn: ["WARN", "WRN", "WARNING"]
  error: ["ERROR", "ERR"]
  fatal: ["FATAL", "FTL", "CRITICAL"]

colors:
  trace: "240"
  debug: "244"
  info: "white"
  warn: "yellow"
  error: "red"
  fatal: "red"
```

## License

MIT

## Contributing

Contributions welcome! Please open an issue or PR on GitHub.
