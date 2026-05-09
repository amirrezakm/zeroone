package main

import (
	"context"
	"encoding/json"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sakhtar/xray-stack-zeroone/internal/api"
	"github.com/sakhtar/xray-stack-zeroone/internal/failover"
	"github.com/sakhtar/xray-stack-zeroone/internal/stack"
)

func main() {
	configPath := flag.String("config", "config/stack.example.json", "stack config path")
	printXray := flag.Bool("print-xray", false, "print generated xray config and exit")
	allowApply := flag.Bool("allow-apply", false, "allow endpoints that modify live Xray/systemd state")
	manageFailover := flag.Bool("manage-failover", false, "run the automatic Xray tunnel failover loop")
	flag.Parse()

	cfg, err := stack.Load(*configPath)
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}
	if *printXray {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(api.GenerateXrayForCLI(*cfg)); err != nil {
			panic(err)
		}
		return
	}

	h := api.NewServer(*cfg, *configPath, *allowApply)
	srv := &http.Server{Addr: cfg.Server.AdminListen, Handler: h, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		slog.Info("xray-stackd listening", "addr", cfg.Server.AdminListen)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server", "err", err)
			os.Exit(1)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if *manageFailover {
		slog.Info("starting failover manager")
		go (&failover.Manager{ConfigPath: *configPath}).Run(ctx)
	}
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
