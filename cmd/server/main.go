package main

import (
	"context"
	"log"
	"os"
	"os/signal"
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
	if err := app.Run(ctx, addr); err != nil {
		log.Fatal(err)
	}
}
