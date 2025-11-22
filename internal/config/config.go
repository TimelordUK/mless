package config

import (
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// Config holds all application configuration
type Config struct {
	Theme       ThemeConfig       `toml:"theme"`
	LogLevels   LogLevelConfig    `toml:"log_levels"`
	Keybindings KeybindingConfig  `toml:"keybindings"`
	Display     DisplayConfig     `toml:"display"`
}

// ThemeConfig defines color schemes
type ThemeConfig struct {
	Name           string          `toml:"name"`
	LineNumbers    string          `toml:"line_numbers"`
	StatusBar      string          `toml:"status_bar"`
	StatusBarText  string          `toml:"status_bar_text"`
	SearchMatch    string          `toml:"search_match"`
	Levels         LogLevelColors  `toml:"levels"`
}

// LogLevelColors defines colors for each log level
type LogLevelColors struct {
	Trace   string `toml:"trace"`
	Debug   string `toml:"debug"`
	Info    string `toml:"info"`
	Warn    string `toml:"warn"`
	Error   string `toml:"error"`
	Fatal   string `toml:"fatal"`
}

// LogLevelConfig defines log level detection patterns
type LogLevelConfig struct {
	TracePatterns []string `toml:"trace_patterns"`
	DebugPatterns []string `toml:"debug_patterns"`
	InfoPatterns  []string `toml:"info_patterns"`
	WarnPatterns  []string `toml:"warn_patterns"`
	ErrorPatterns []string `toml:"error_patterns"`
	FatalPatterns []string `toml:"fatal_patterns"`
}

// KeybindingConfig allows customizing keybindings
type KeybindingConfig struct {
	Quit       []string `toml:"quit"`
	ScrollUp   []string `toml:"scroll_up"`
	ScrollDown []string `toml:"scroll_down"`
	PageUp     []string `toml:"page_up"`
	PageDown   []string `toml:"page_down"`
	Top        []string `toml:"top"`
	Bottom     []string `toml:"bottom"`
	Search     []string `toml:"search"`
	NextMatch  []string `toml:"next_match"`
	PrevMatch  []string `toml:"prev_match"`
}

// DisplayConfig holds display options
type DisplayConfig struct {
	ShowLineNumbers bool `toml:"show_line_numbers"`
	TabWidth        int  `toml:"tab_width"`
	WrapLines       bool `toml:"wrap_lines"`
}

// DefaultConfig returns a config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Theme: ThemeConfig{
			Name:          "subtle",
			LineNumbers:   "240",      // Dark gray
			StatusBar:     "236",      // Darker gray background
			StatusBarText: "252",      // Light gray text
			SearchMatch:   "226",      // Yellow
			Levels: LogLevelColors{
				Trace: "240",   // Dark gray
				Debug: "244",   // Medium gray
				Info:  "250",   // Light gray (default)
				Warn:  "214",   // Orange
				Error: "167",   // Soft red
				Fatal: "196",   // Bright red
			},
		},
		LogLevels: LogLevelConfig{
			TracePatterns: []string{"[TRC]", "[TRACE]", "TRACE", "TRC"},
			DebugPatterns: []string{"[DBG]", "[DEBUG]", "DEBUG", "DBG"},
			InfoPatterns:  []string{"[INF]", "[INFO]", "INFO", "INF"},
			WarnPatterns:  []string{"[WRN]", "[WARN]", "[WARNING]", "WARN", "WRN", "WARNING"},
			ErrorPatterns: []string{"[ERR]", "[ERROR]", "ERROR", "ERR"},
			FatalPatterns: []string{"[FTL]", "[FATAL]", "FATAL", "FTL", "[CRIT]", "CRITICAL"},
		},
		Keybindings: KeybindingConfig{
			Quit:       []string{"q", "ctrl+c"},
			ScrollUp:   []string{"k", "up"},
			ScrollDown: []string{"j", "down"},
			PageUp:     []string{"b", "pgup", "ctrl+u"},
			PageDown:   []string{"f", "pgdown", "ctrl+d", " "},
			Top:        []string{"g", "home"},
			Bottom:     []string{"G", "end"},
			Search:     []string{"/"},
			NextMatch:  []string{"n"},
			PrevMatch:  []string{"N"},
		},
		Display: DisplayConfig{
			ShowLineNumbers: true,
			TabWidth:        4,
			WrapLines:       false,
		},
	}
}

// Load loads config from file, falling back to defaults
func Load() (*Config, error) {
	cfg := DefaultConfig()

	// Try to load from config file
	configPath := getConfigPath()
	if configPath == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}

	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Save saves config to file
func Save(cfg *Config) error {
	configPath := getConfigPath()
	if configPath == "" {
		return nil
	}

	// Ensure directory exists
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}

// getConfigPath returns the config file path
func getConfigPath() string {
	// Check XDG_CONFIG_HOME first
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "mless", "config.toml")
	}

	// Fall back to ~/.config
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(home, ".config", "mless", "config.toml")
}

// GetConfigPath exports the config path for user reference
func GetConfigPath() string {
	return getConfigPath()
}
