package game

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// HunterSave contains only game progress; no personal financial documents.
// The seed and property IDs pin a session to the original cached listing order.
type HunterSave struct {
	Version     int      `json:"version"`
	Mode        string   `json:"mode"`
	City        string   `json:"city"`
	Source      string   `json:"source"`
	Seed        int64    `json:"seed"`
	Rounds      int      `json:"rounds"`
	Index       int      `json:"index"`
	Turns       int      `json:"turns"`
	Score       int      `json:"score"`
	Viewed      bool     `json:"viewed"`
	Inspected   bool     `json:"inspected"`
	PropertyIDs []string `json:"property_ids"`
	// Prices pin the answers and asking prices to the original game session.
	// Legacy saves without prices can still load but cannot verify price changes.
	Prices []int64 `json:"prices,omitempty"`
}

func DefaultHunterSavePath() (string, error) {
	h, err := DefaultHistoryPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(h), "hunter-save.json"), nil
}

func (s HunterSave) Validate() error {
	if s.Version != 1 || (s.Mode != "buy" && s.Mode != "rent") || s.Rounds < 1 || s.Rounds > 100 || s.Index < 0 || s.Index >= s.Rounds || s.Turns < 0 || s.Turns > 20 || s.Score < 0 || len(s.PropertyIDs) != s.Rounds {
		return errors.New("invalid hunter save")
	}
	if len(s.Prices) != 0 && len(s.Prices) != s.Rounds {
		return errors.New("invalid saved price snapshot length")
	}
	for _, price := range s.Prices {
		if price <= 0 {
			return errors.New("invalid saved asking price")
		}
	}
	for _, id := range s.PropertyIDs {
		if strings.TrimSpace(id) == "" {
			return errors.New("empty property ID")
		}
	}
	if s.Inspected && !s.Viewed {
		return errors.New("inspection without viewing")
	}
	return nil
}

func LoadHunter(path string) (HunterSave, error) {
	var s HunterSave
	b, err := os.ReadFile(path)
	if err != nil {
		return s, err
	}
	if err = json.Unmarshal(b, &s); err != nil {
		return s, fmt.Errorf("parse hunter save: %w", err)
	}
	if err = s.Validate(); err != nil {
		return s, err
	}
	return s, nil
}

func SaveHunter(path string, s HunterSave) error {
	if err := s.Validate(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".hunter-save-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err = f.Chmod(0600); err != nil {
		return err
	}
	if _, err = f.Write(append(b, '\n')); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
