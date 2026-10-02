package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/logic3579/burrow/internal/burrow"
	"github.com/logic3579/burrow/web"
)

func main() {
	if err := run(); err != nil {
		slog.Error("burrow failed", "error", err)
		os.Exit(1)
	}
}
func run() error {
	command := "serve"
	args := os.Args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		command, args = args[0], args[1:]
	}
	switch command {
	case "serve", "migrate", "seed", "keys-rotate", "healthcheck", "mfa-reset":
	default:
		return fmt.Errorf("unknown command %q", command)
	}
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	configPath := fs.String("config", "", "configuration file (default: configs/config.yaml or embedded defaults)")
	var username, reason *string
	if command == "mfa-reset" {
		username = fs.String("username", "", "account to reset")
		reason = fs.String("reason", "", "required audit reason")
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if command == "healthcheck" {
		url := os.Getenv("BURROW_HEALTHCHECK_URL")
		if url == "" {
			url = "http://127.0.0.1:8080/readyz"
		}
		client := http.Client{Timeout: 3 * time.Second}
		r, e := client.Get(url)
		if e != nil {
			return e
		}
		defer r.Body.Close()
		if r.StatusCode != 200 {
			return fmt.Errorf("not ready: %d", r.StatusCode)
		}
		return nil
	}
	c, e := burrow.LoadConfigFile(*configPath)
	if e != nil {
		return e
	}
	var store *burrow.Store
	for i := 0; i < 15; i++ {
		store, e = burrow.Open(c)
		if e == nil {
			break
		}
		if command != "migrate" {
			return e
		}
		time.Sleep(2 * time.Second)
	}
	if e != nil {
		return e
	}
	sql, e := store.DB.DB()
	if e != nil {
		return e
	}
	defer sql.Close()
	switch command {
	case "migrate":
		if e = store.Migrate(); e != nil {
			return e
		}
		var n int64
		if e = store.DB.Model(&burrow.SigningKey{}).Where("active = ?", true).Count(&n).Error; e != nil {
			return e
		}
		if n == 0 {
			return store.RotateKeys()
		}
		return nil
	case "seed":
		if e := store.Seed(c.Bootstrap); e != nil {
			return e
		}
		slog.Info("seed completed; existing administrator credentials preserved")
		return nil
	case "mfa-reset":
		if e = store.Health(context.Background()); e != nil {
			return e
		}
		if e = store.ResetMFA(*username, *reason); e != nil {
			return e
		}
		slog.Info("MFA reset completed; next password login requires enrollment")
		return nil
	case "keys-rotate":
		if e = store.Health(context.Background()); e != nil {
			return e
		}
		return store.RotateKeys()
	case "serve":
		server, e := burrow.NewServer(store, web.Assets())
		if e != nil {
			return e
		}
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		go store.Cleanup(ctx)
		h := &http.Server{Addr: c.ListenAddr, Handler: server, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
		go func() {
			<-ctx.Done()
			shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			h.Shutdown(shutdown)
		}()
		slog.Info("burrow listening", "address", c.ListenAddr)
		e = h.ListenAndServe()
		if e == http.ErrServerClosed {
			return nil
		}
		return e
	default:
		return fmt.Errorf("unknown command %q", command)
	}
}
