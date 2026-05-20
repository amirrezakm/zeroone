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

	"github.com/amirrezakm/zeroone/internal/analytics"
	"github.com/amirrezakm/zeroone/internal/api"
	"github.com/amirrezakm/zeroone/internal/audit"
	"github.com/amirrezakm/zeroone/internal/enforce"
	"github.com/amirrezakm/zeroone/internal/events"
	"github.com/amirrezakm/zeroone/internal/failover"
	"github.com/amirrezakm/zeroone/internal/metrics"
	"github.com/amirrezakm/zeroone/internal/monitor"
	"github.com/amirrezakm/zeroone/internal/notify"
	"github.com/amirrezakm/zeroone/internal/presence"
	"github.com/amirrezakm/zeroone/internal/relay"
	"github.com/amirrezakm/zeroone/internal/snapshots"
	"github.com/amirrezakm/zeroone/internal/stack"
	"github.com/amirrezakm/zeroone/internal/system"
	"github.com/amirrezakm/zeroone/internal/tunnel"
	"github.com/amirrezakm/zeroone/internal/usage"
	xrayinternal "github.com/amirrezakm/zeroone/internal/xray"
	"github.com/amirrezakm/zeroone/internal/xrayproc"
)

func main() {
	// Subcommand dispatch (zeroone admin add/list/reset-password).
	// Must run before flag.Parse so the subcommand's own flag set owns
	// the remaining argv. The default verb is "run" (the daemon).
	if handled, code := runAdminSubcommand(os.Args[1:]); handled {
		os.Exit(code)
	}

	configPath := flag.String("config", "config/stack.example.json", "stack config path")
	printXray := flag.Bool("print-xray", false, "print generated xray config and exit")
	allowApply := flag.Bool("allow-apply", false, "allow endpoints that modify live Xray/systemd state")
	manageFailover := flag.Bool("manage-failover", false, "run the automatic Xray tunnel failover loop")
	manageVPN := flag.Bool("manage-vpn", false, "restart tunnel services when their unit or interface goes down")
	manageRelay := flag.Bool("manage-relay", false, "supervise the mhrv-rs relay plugin (auto-start, restart, probe)")
	manageXray := flag.Bool("manage-xray", false, "run xray as a child process and restart it on apply (container mode; replaces systemctl)")
	flag.Parse()

	// Auto-init: if -config points at a missing file and ZEROONE_AUTO_INIT=1,
	// write a minimal default config so the daemon can start fresh.
	// Container builds set this; host installs leave it unset.
	if os.Getenv("ZEROONE_AUTO_INIT") == "1" {
		if _, err := os.Stat(*configPath); os.IsNotExist(err) {
			if err := writeMinimalConfig(*configPath); err != nil {
				slog.Error("auto-init config", "err", err)
				os.Exit(1)
			}
			slog.Info("auto-init wrote minimal config", "path", *configPath)
		}
	}

	cfg, err := stack.Load(*configPath)
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}
	applyEnvOverrides(cfg)
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
		stateDir = "/var/lib/zeroone"
	}
	migrateLegacyStateDir(stateDir)
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
		slog.Info("zeroone listening", "addr", cfg.Server.AdminListen)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server", "err", err)
			os.Exit(1)
		}
	}()

	ctx := ctxRoot
	if *manageXray {
		// Run xray as a child process; wire it in as the active
		// Restarter so api.Apply uses it instead of `systemctl`.
		xrayLogPath := filepath.Join(stateDir, "logs", "xray.log")
		sup := xrayproc.New(cfg.Server.XrayBinary, cfg.Server.XrayConfigPath, xrayLogPath, slog.Default())
		xrayinternal.SetRestarter(sup)
		slog.Info("starting xray supervisor", "binary", cfg.Server.XrayBinary, "config", cfg.Server.XrayConfigPath, "log", xrayLogPath)
		go sup.Run(ctx)
	}
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
