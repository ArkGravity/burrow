package main

import (
	"bufio"
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
	if len(os.Args) > 1 {
		command = os.Args[1]
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
	c, e := burrow.LoadConfig()
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
	case "admin-init":
		fs := flag.NewFlagSet("admin-init", flag.ContinueOnError)
		fs.Bool("password-stdin", true, "read password from stdin")
		username := fs.String("username", "admin", "initial administrator username")
		if e = fs.Parse(os.Args[2:]); e != nil {
			return e
		}
		fmt.Fprintln(os.Stderr, "Read initial administrator password from stdin (minimum 12 characters):")
		scanner := bufio.NewScanner(os.Stdin)
		if !scanner.Scan() {
			return fmt.Errorf("password required on stdin")
		}
		return store.InitAdmin(strings.TrimSpace(*username), scanner.Text())
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
