package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"aegis/internal/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.RunWorker(ctx); err != nil {
		log.Fatal(err)
	}
}
