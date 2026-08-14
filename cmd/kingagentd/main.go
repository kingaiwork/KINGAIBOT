package main

import (
	"context"
	"crypto/subtle"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/agent"
	"github.com/kingaiwork/KINGAIBOT/internal/api"
	"github.com/kingaiwork/KINGAIBOT/internal/approval"
	"github.com/kingaiwork/KINGAIBOT/internal/config"
	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
	"github.com/kingaiwork/KINGAIBOT/internal/evolution"
	"github.com/kingaiwork/KINGAIBOT/internal/memory"
	"github.com/kingaiwork/KINGAIBOT/internal/platform"
	"github.com/kingaiwork/KINGAIBOT/internal/policy"
	"github.com/kingaiwork/KINGAIBOT/internal/provider"
	karuntime "github.com/kingaiwork/KINGAIBOT/internal/runtime"
	"github.com/kingaiwork/KINGAIBOT/internal/task"
	"github.com/kingaiwork/KINGAIBOT/internal/tool"
)

var version = "1.3.0"

func main() {
	cfgPath := flag.String("config", "config.json", "configuration file")
	showVersion := flag.Bool("version", false, "print version")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		slog.Error("config", "error", err)
		os.Exit(1)
	}
	cfg.Version = version
	ts, mustErr := task.NewStore(filepath.Join(cfg.Runtime.DataDir, "tasks"))
	must(mustErr)
	as, mustErr := approval.New(filepath.Join(cfg.Runtime.DataDir, "approvals"))
	must(mustErr)
	el, mustErr := eventlog.New(filepath.Join(cfg.Runtime.DataDir, "events"))
	must(mustErr)
	ms, mustErr := memory.New(filepath.Join(cfg.Runtime.DataDir, "memory"), cfg.Memory.MaxRecords)
	must(mustErr)
	es, mustErr := evolution.New(filepath.Join(cfg.Runtime.DataDir, "evolution"))
	must(mustErr)
	pe := policy.New(cfg.Security.DefaultToolPolicy, cfg.Security.ToolPolicies)
	tr := tool.New(cfg, pe, as, el)
	pc := provider.New(cfg.Providers, time.Duration(cfg.Runtime.RequestTimeoutSeconds)*time.Second)
	ae := agent.New(cfg, pc, tr, ms)
	rt := karuntime.New(ts, as, el, ms, ae, es, cfg)
	defer rt.Close()

	pm, mustErr := platform.New(filepath.Join(cfg.Runtime.DataDir, "platform"), rt, el)
	must(mustErr)
	defer pm.Close()
	tr.RegisterExtension(pm)

	must(rt.Recover())

	coreHandler := api.New(cfg, rt, tr).Handler()
	root := http.NewServeMux()
	root.Handle("/v1/platform/", bearerEnv(cfg.Server.AdminTokenEnv, pm.Handler()))
	root.Handle("/", coreHandler)

	srv := &http.Server{Addr: cfg.Server.Listen, Handler: root, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: time.Duration(cfg.Runtime.RequestTimeoutSeconds+15) * time.Second, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 32 << 10}
	go func() {
		slog.Info("KINGAIBOT started", "listen", cfg.Server.Listen, "version", version, "platform", "enabled")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server", "error", err)
			os.Exit(1)
		}
	}()
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	slog.Info("shutting down")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)
}

func bearerEnv(envName string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expected := os.Getenv(envName)
		if len(expected) < 32 {
			http.Error(w, "server authentication is not configured", http.StatusInternalServerError)
			return
		}
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			http.Error(w, "bearer token required", http.StatusUnauthorized)
			return
		}
		got := strings.TrimPrefix(auth, "Bearer ")
		if len(got) != len(expected) || subtle.ConstantTimeCompare([]byte(got), []byte(expected)) != 1 {
			http.Error(w, "valid bearer token required", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func must(err error) {
	if err != nil {
		slog.Error("startup failed", "error", err)
		os.Exit(1)
	}
}
