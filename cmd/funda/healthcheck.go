package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

// runHealth checks that a successful monitoring cycle completed recently.
// It intentionally does not require an HTTP listener.
func runHealth(args []string) error {
	fs := flag.NewFlagSet("health", flag.ContinueOnError)
	maxAge := fs.Duration("max-age", 30*time.Minute, "maximum time since a successful monitoring cycle")
	file := fs.String("file", os.Getenv("FUNDA_HEALTH_FILE"), "successful-check timestamp file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *maxAge <= 0 {
		return fmt.Errorf("max-age must be positive")
	}
	if *file == "" {
		return fmt.Errorf("FUNDA_HEALTH_FILE or --file is required")
	}
	b, err := os.ReadFile(*file)
	if err != nil {
		return fmt.Errorf("read monitoring heartbeat: %w", err)
	}
	last, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(string(b)))
	if err != nil {
		return fmt.Errorf("invalid monitoring heartbeat: %w", err)
	}
	age := time.Since(last)
	if age < -time.Minute || age > *maxAge {
		return fmt.Errorf("last successful check at %s (%s ago), outside allowed age %s", last.Format(time.RFC3339), age.Round(time.Second), *maxAge)
	}
	fmt.Printf("healthy: last successful check %s (%s ago)\n", last.Format(time.RFC3339), age.Round(time.Second))
	return nil
}
