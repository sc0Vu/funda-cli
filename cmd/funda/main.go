package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/sc0vu/funda-cli/internal/app"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	dbPath := os.Getenv("FUNDA_DB")
	if dbPath == "" {
		dbPath = "funda.db"
	}

	if len(os.Args) > 1 && os.Args[1] == "health" {
		if err := runHealth(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "unhealthy:", err)
			os.Exit(1)
		}
		return
	}

	if os.Getenv("FUNDA_ENABLE_HTTP_HEALTH") == "1" && len(os.Args) > 1 && os.Args[1] == "watch" && (len(os.Args) == 2 || os.Args[2] != "list") {
		if err := startHealthServer(ctx, dbPath); err != nil {
			fmt.Fprintln(os.Stderr, "health server:", err)
			os.Exit(1)
		}
	}

	if err := (app.App{DBPath: dbPath}).Run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
