// Package gameimage detects terminal image support conservatively. The game
// currently has no licensed photo URLs in its normalized listing model.
package gameimage

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
)

type Mode string

const (
	Auto   Mode = "auto"
	Kitty  Mode = "kitty"
	ITerm2 Mode = "iterm2"
	Link   Mode = "link"
	None   Mode = "none"
)

func Parse(s string) (Mode, error) {
	switch Mode(strings.ToLower(strings.TrimSpace(s))) {
	case Auto, Kitty, ITerm2, Link, None:
		return Mode(strings.ToLower(strings.TrimSpace(s))), nil
	default:
		return "", fmt.Errorf("unknown image mode %q (auto|kitty|iterm2|link|none)", s)
	}
}

// Detect uses environment hints, not a protocol handshake. It intentionally
// falls back to links for tmux, screen, SSH, or unrecognized terminals.
func Detect(env map[string]string, interactive bool) Mode {
	if !interactive || env["TERM"] == "dumb" || env["TMUX"] != "" || strings.HasPrefix(env["TERM"], "screen") {
		return Link
	}
	if env["KITTY_WINDOW_ID"] != "" {
		return Kitty
	}
	if env["TERM_PROGRAM"] == "iTerm.app" {
		return ITerm2
	}
	return Link
}
func Environment() map[string]string {
	keys := []string{"TERM", "TMUX", "KITTY_WINDOW_ID", "TERM_PROGRAM"}
	m := make(map[string]string, len(keys))
	for _, k := range keys {
		m[k] = os.Getenv(k)
	}
	return m
}
func Resolve(requested Mode, env map[string]string, interactive bool) Mode {
	if requested != Auto {
		return requested
	}
	return Detect(env, interactive)
}
func SafePhotoURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
		return errors.New("photo must have a public HTTPS URL")
	}
	if strings.ContainsAny(raw, "\r\n\x1b") {
		return errors.New("unsafe URL")
	}
	return nil
}
