package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/opemori/graybox-core/internal/cli"
)

// version can be replaced at build time with:
// go build -ldflags "-X main.version=v0.1.0" ./cmd/graybox
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	app := cli.App{Stdout: os.Stdout, Stderr: os.Stderr, Version: version}
	os.Exit(app.Run(ctx, os.Args[1:]))
}
