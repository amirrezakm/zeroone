package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/sakhtar/xray-stack-zeroone/internal/analytics"
	"github.com/sakhtar/xray-stack-zeroone/internal/api"
	"github.com/sakhtar/xray-stack-zeroone/internal/audit"
	"github.com/sakhtar/xray-stack-zeroone/internal/enforce"
	"github.com/sakhtar/xray-stack-zeroone/internal/events"
	"github.com/sakhtar/xray-stack-zeroone/internal/failover"
	"github.com/sakhtar/xray-stack-zeroone/internal/metrics"
	"github.com/sakhtar/xray-stack-zeroone/internal/monitor"
	"github.com/sakhtar/xray-stack-zeroone/internal/notify"
	"github.com/sakhtar/xray-stack-zeroone/internal/presence"
	"github.com/sakhtar/xray-stack-zeroone/internal/relay"
	"github.com/sakhtar/xray-stack-zeroone/internal/snapshots"
	"github.com/sakhtar/xray-stack-zeroone/internal/stack"
	"github.com/sakhtar/xray-stack-zeroone/internal/system"
	"github.com/sakhtar/xray-stack-zeroone/internal/tunnel"
	"github.com/sakhtar/xray-stack-zeroone/internal/usage"
)

func main() {
	configPath := flag.String("config", "config/stack.example.json", "stack config path")
	printXray := flag.Bool("print-xray", false, "print generated xray config and exit")
	allowApply := flag.Bool("allow-apply", false, "allow endpoints that modify live Xray/systemd state")
	manageFailover := flag.Bool("manage-failover", false, "run the automatic Xray tunnel failover loop")
	manageVPN := flag.Bool("manage-vpn", false, "restart tunnel services when their unit or interface goes down")
	manageRelay := flag.Bool("manage-relay", false, "supervise the mhrv-rs relay plugin (auto-start, restart, probe)")
	flag.Parse()

	cfg, err := stack.Load(*configPath)
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}
	// Backfill subscription tokens for any user that doesn't have one yet.
	// Persisted immediately so /sub/{token} and /me/{token} keep working
	// across restarts. Skipped in -print-xray mode (read-only operation).
	if !*printXray && cfg.EnsureSubTokens() {
		if err := stack.Save(*configPath, *cfg); err != nil {
			slog.Error("persist backfilled sub tokens", "err", err)
			os.Exit(1)
		}
		slog.Info("backfilled subscription tokens for users without one")
	}
	if *printXray {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(api.GenerateXrayForCLI(*cfg)); err != nil {
			panic(err)
		}
		return
	}

	stateDir := filepath.Dir(cfg.Server.FailoverStatePath)
	if stateDir == "" || stateDir == "." {
		stateDir = "/var/lib/xray-stack"
	}
	auditLog := audit.New(filepath.Join(stateDir, "audit.log"))
	snapStore := snapshots.New(filepath.Join(stateDir, "snapshots"))
	presenceTracker := presence.New(filepath.Join(stateDir, "presence.json"))
	broker := events.NewBroker()
	store := metrics.NewStore(
		func() stack.Config {
			c, err := stack.Load(*configPath)
			if err != nil {
				return *cfg
			}
			return *c
		},
		func() string {
			c, err := stack.Load(*configPath)
			if err != nil {
				return failover.CurrentMode(*cfg).OutboundTag
			}
			return failover.CurrentMode(*c).OutboundTag
		},
	)

	ctxRoot, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go store.Run(ctxRoot)
	go (&notify.Notifier{Broker: broker, ConfigRead: func() stack.Config {
		c, err := stack.Load(*configPath)
		if err != nil {
			return *cfg
		}
		return *c
	}}).Run(ctxRoot)
	go presenceTracker.Run(ctxRoot, func() stack.Config {
		c, err := stack.Load(*configPath)
		if err != nil {
			return *cfg
		}
		return *c
	})
	go usage.Run(ctxRoot, func() usage.SyncConfig {
		c, err := stack.Load(*configPath)
		if err != nil {
			c = cfg
		}
		port := c.Xray.APIPort
		if port == 0 {
			port = 10085
		}
		return usage.SyncConfig{
			Path:       c.Server.UserUsagePath,
			APIAddress: fmt.Sprintf("127.0.0.1:%d", port),
		}
	}, store.ObserveUserStats)
	go usage.RunResetLoop(ctxRoot, func() usage.ResetConfig {
		c, err := stack.Load(*configPath)
		if err != nil {
			c = cfg
		}
		users := make([]usage.PeriodUser, 0, len(c.Xray.Users))
		for _, u := range c.Xray.Users {
			users = append(users, usage.PeriodUser{
				Email:          u.Email,
				DailyResetHHMM: u.EffectiveDailyResetHHMM(),
			})
		}
		return usage.ResetConfig{Path: c.Server.UserUsagePath, Users: users}
	})

	destPath := cfg.Server.DestinationsPath
	if destPath == "" {
		destPath = filepath.Join(stateDir, "destinations.json")
	}
	destAgg := analytics.New(destPath)
	go destAgg.Run(ctxRoot)

	relayStore := relay.NewStore()
	relayLoader := func() (stack.RelayConfig, error) {
		c, err := stack.Load(*configPath)
		if err != nil {
			return stack.RelayConfig{}, err
		}
		return c.Relay, nil
	}
	var relaySupervisor *relay.Supervisor
	if *manageRelay {
		relaySupervisor = relay.NewSupervisor(*configPath, relayLoader, relayStore, broker)
	}

	h := api.NewServerWithOptions(*cfg, *configPath, *allowApply, api.Options{
		Metrics:         store,
		Events:          broker,
		Audit:           auditLog,
		Snapshots:       snapStore,
		Presence:        presenceTracker,
		Destinations:    destAgg,
		RelayStore:      relayStore,
		RelaySupervisor: relaySupervisor,
	})
	srv := &http.Server{
		Addr:              cfg.Server.AdminListen,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	go func() {
		slog.Info("xray-stackd listening", "addr", cfg.Server.AdminListen)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server", "err", err)
			os.Exit(1)
		}
	}()

	ctx := ctxRoot
	if *manageFailover {
		slog.Info("starting failover manager")
		go (&failover.Manager{ConfigPath: *configPath}).Run(ctx)
	}
	if *manageVPN {
		slog.Info("starting tunnel supervisor")
		go (&tunnel.Supervisor{ConfigPath: *configPath}).Run(ctx)
	}
	if relaySupervisor != nil {
		slog.Info("starting relay supervisor")
		go relaySupervisor.Run(ctx)
	}
	if *allowApply {
		slog.Info("starting session-limit enforcer")
		go enforce.RunSessionEnforcer(ctx,
			func() enforce.EnforceConfig {
				c, err := stack.Load(*configPath)
				if err != nil {
					c = cfg
				}
				return enforce.EnforceConfig{Cfg: *c, Ports: xrayInboundPorts(*c)}
			},
			func(snapCtx context.Context) (monitor.OnlineSnapshot, error) {
				c, err := stack.Load(*configPath)
				if err != nil {
					return monitor.OnlineSnapshot{}, err
				}
				return monitor.Online(snapCtx, nil, 300, xrayInboundPorts(*c))
			},
		)
	}
	system.SDNotifyReady()
	go system.RunWatchdog(ctx)
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}

// xrayInboundPorts mirrors api.Server.xrayInboundPorts() for use from the
// background loops in this package. The xhttp port matters most on this
// stack — that's where nginx hands off CDN traffic — without it, the
// session enforcer never sees any peers and `ss`-based active counts are
// always zero.
func xrayInboundPorts(c stack.Config) []int {
	ports := []int{}
	if p := c.Xray.Inbounds.VLESSWSPort; p != 0 {
		ports = append(ports, p)
	}
	if p := c.Xray.Inbounds.VLESSXHTTPPort; p != 0 {
		ports = append(ports, p)
	}
	for _, sock := range c.Xray.Inbounds.PublicSOCKS {
		if sock.Port != 0 {
			ports = append(ports, sock.Port)
		}
	}
	for _, u := range c.Xray.Users {
		if u.BandwidthPort != 0 {
			ports = append(ports, u.BandwidthPort)
		}
	}
	return ports
}
