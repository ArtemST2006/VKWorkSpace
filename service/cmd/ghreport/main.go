// Package main starts the ghreport command.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"git.miem.hse.ru/blank5204543/vkworkspace/service/internal/app"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return app.Run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr)
}
