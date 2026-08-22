package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"syscall"

	"emos_video_upload/api"
)

var (
	version    = "dev"
	buildTime  = ""
	gitVersion = "dev"
)

// The production build places the Vite output under web/dist before compiling.
//
//go:embed web/dist
var frontend embed.FS

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Printf("version: %s\nbuild_time: %s\ngit_version: %s\n", version, buildTime, gitVersion)
		return
	}
	if len(os.Args) > 1 && os.Args[1] != "server" {
		fmt.Fprintf(os.Stderr, "unknown command %q; use version or server\n", os.Args[1])
		os.Exit(2)
	}

	cfg, err := api.LoadConfig("")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	staticFS, err := fs.Sub(frontend, "web/dist")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	server, err := api.NewServer(cfg, staticFS)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer server.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := server.Serve(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
