package enrich

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/sc0vu/funda-cli/internal/model"
)

type CommandProvider struct {
	Command string
}

func (p CommandProvider) Fetch(ctx context.Context, rawURL string) (model.Listing, error) {
	var out model.Listing
	parts := strings.Fields(p.Command)
	if len(parts) == 0 {
		return out, fmt.Errorf("empty enrichment command")
	}
	cmd := exec.CommandContext(ctx, parts[0], append(parts[1:], rawURL)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return out, fmt.Errorf("detail provider failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return out, fmt.Errorf("decode provider JSON: %w", err)
	}
	return out, nil
}
