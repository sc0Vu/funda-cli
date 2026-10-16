package game

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// SessionResult stores a completed game summary, never an unrevealed answer.
type SessionResult struct {
	Game        string    `json:"game"`
	Mode        string    `json:"mode"`
	City        string    `json:"city"`
	Seed        int64     `json:"seed"`
	Rounds      int       `json:"rounds"`
	Score       int       `json:"score"`
	CompletedAt time.Time `json:"completed_at"`
}

// DefaultHistoryPath returns a per-user data location for completed game results.
func DefaultHistoryPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	if home == "" {
		return "", errors.New("home directory unavailable")
	}
	var dir string
	switch runtime.GOOS {
	case "darwin":
		dir = filepath.Join(home, "Library", "Application Support")
	case "windows":
		dir = os.Getenv("APPDATA")
		if dir == "" {
			dir = filepath.Join(home, "AppData", "Roaming")
		}
	default:
		dir = os.Getenv("XDG_DATA_HOME")
		if dir == "" || !filepath.IsAbs(dir) {
			dir = filepath.Join(home, ".local", "share")
		}
	}
	return filepath.Join(dir, "funda", "game-history.json"), nil
}

// AppendResult writes the session history atomically. History is local and
// contains no listing identifiers, prices, photos, or addresses.
func AppendResult(path string, result SessionResult) error {
	if path == "" {
		return nil
	}
	if result.Game != "guess" && result.Game != "hunter" {
		return errors.New("invalid game")
	}
	if result.Mode != "buy" && result.Mode != "rent" {
		return errors.New("invalid game mode")
	}
	if result.Rounds < 0 || result.Score < 0 {
		return errors.New("invalid game result")
	}
	var entries []SessionResult
	data, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(data, &entries); err != nil {
			return fmt.Errorf("read game history: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	entries = append(entries, result)
	data, err = json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".funda-history-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err := f.Chmod(0600); err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
