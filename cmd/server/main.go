package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"probe.local/monitor/internal/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	addr := os.Getenv("TINYPROBE_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	databasePath := strings.TrimSpace(os.Getenv("TINYPROBE_DB_PATH"))
	if databasePath == "" {
		databasePath = "tinyprobe.db"
	}
	if err := app.Run(ctx, addr, app.Config{
		DatabasePath: databasePath,
		AgentFiles:   os.DirFS("downloads"),
	}); err != nil {
		log.Fatal(err)
	}
}
