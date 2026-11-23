package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/onlyin32bit/4t/internal/app"
	"github.com/onlyin32bit/4t/internal/config"
	applog "github.com/onlyin32bit/4t/internal/shared/logger"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "4t:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		return err
	}
	if err := applog.Configure(cfg.Server.LogLevel); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	application, err := app.New(ctx, cfg, os.Stdin, os.Stdout)
	if err != nil {
		return err
	}
	defer application.Close()

	return application.Run(ctx)
}
