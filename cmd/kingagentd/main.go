package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/agent"
	"github.com/kingaiwork/KINGAIBOT/internal/api"
	"github.com/kingaiwork/KINGAIBOT/internal/approval"
	"github.com/kingaiwork/KINGAIBOT/internal/authority"
	"github.com/kingaiwork/KINGAIBOT/internal/cluster"
	"github.com/kingaiwork/KINGAIBOT/internal/cognition"
	"github.com/kingaiwork/KINGAIBOT/internal/config"
	"github.com/kingaiwork/KINGAIBOT/internal/eventlog"
	"github.com/kingaiwork/KINGAIBOT/internal/evolution"
	"github.com/kingaiwork/KINGAIBOT/internal/knowledge"
	"github.com/kingaiwork/KINGAIBOT/internal/memory"
	"github.com/kingaiwork/KINGAIBOT/internal/orchestration"
	"github.com/kingaiwork/KINGAIBOT/internal/platform"
	"github.com/kingaiwork/KINGAIBOT/internal/policy"
	"github.com/kingaiwork/KINGAIBOT/internal/provider"
	karuntime "github.com/kingaiwork/KINGAIBOT/internal/runtime"
	"github.com/kingaiwork/KINGAIBOT/internal/task"
	"github.com/kingaiwork/KINGAIBOT/internal/tool"
	"github.com/kingaiwork/KINGAIBOT/internal/workgraph"
)

var version = "1.6.0"

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
	ec, mustErr := evolution.NewController(es, el)
	must(mustErr)
	ce, mustErr := cognition.New(filepath.Join(cfg.Runtime.DataDir, "cognition"), ms, ec, el, cognition.Config{
		Enabled:                      cfg.Memory.Enabled,
		ReflectionInterval:           30 * time.Minute,
		MaxPrinciples:                32,
		AutoProposalFailureThreshold: 3,
		StoreTaskInputs:              cfg.Memory.StoreTaskInputs,
	})
	must(mustErr)
	defer ce.Close()
	must(ce.AttachTasks(ts))
	pe := policy.New(cfg.Security.DefaultToolPolicy, cfg.Security.ToolPolicies)
	tr := tool.New(cfg, pe, as, el)
	pc := provider.New(cfg.Providers, time.Duration(cfg.Runtime.RequestTimeoutSeconds)*time.Second)
	ae := agent.New(cfg, pc, tr, ms)
	ae.SetSelfContextProvider(ce.Context)
	rt := karuntime.New(ts, as, el, ms, ae, es, cfg)
	defer rt.Close()

	// Authority exists before platform task creation so trusted agent identity
	// can be resolved into durable task metadata. Models never provide this ID.
	authorityStore, mustErr := authority.NewStore(filepath.Join(cfg.Runtime.DataDir, "authority"), el)
	must(mustErr)
	boundPlatformRuntime, mustErr := authority.NewBoundTaskRuntime(rt, authorityStore)
	must(mustErr)
	taskAuthorityResolver, mustErr := authority.NewTaskAuthorityResolver(ts)
	must(mustErr)

	// Production uses the crash-safe Platform control surface. Trust-expanding
	// resources are inert until their authorization audit is durable; schedules
	// and recovered workflows also require an audit gate before creating tasks.
	pm, mustErr := platform.NewSafe(filepath.Join(cfg.Runtime.DataDir, "platform"), boundPlatformRuntime, el)
	must(mustErr)
	defer pm.Close()
	platformExtension, mustErr := platform.NewV14Extension(pm)
	must(mustErr)
	tr.RegisterExtension(platformExtension)

	ks, mustErr := knowledge.New(filepath.Join(cfg.Runtime.DataDir, "knowledge"), el)
	must(mustErr)
	knowledgeExtension, mustErr := knowledge.NewSafeExtension(ks)
	must(mustErr)
	tr.RegisterExtension(knowledgeExtension)

	cc, mustErr := cluster.New(filepath.Join(cfg.Runtime.DataDir, "cluster"), el)
	must(mustErr)
	must(cc.SetAuthorityChecker(authorityStore))
	must(cc.SetTaskAuthorityResolver(taskAuthorityResolver))
	tr.RegisterExtension(cc)

	tr.RegisterExtension(ec)

	wgs, mustErr := workgraph.NewStore(filepath.Join(cfg.Runtime.DataDir, "workgraphs"), el)
	must(mustErr)
	orchestrator, mustErr := orchestration.New(filepath.Join(cfg.Runtime.DataDir, "orchestration"), wgs, cc, authorityStore, el)
	must(mustErr)
	defer orchestrator.Close()

	must(rt.Recover())

	coreServer := api.New(cfg, rt, tr)
	coreHandler := coreServer.Handler()
	root := http.NewServeMux()

	// Runtime reconciliation and staged approval decisions are exact admin-only
	// routes. They stay outside MCP/A2A/model tools because ambiguous side effects
	// and trust expansion require operator review.
	root.Handle("POST /v1/tasks/{id}/reconcile", coreServer.TaskReconciliationHandler())
	approvalV14 := coreServer.ApprovalDecisionV14Handler()
	root.Handle("POST /v1/approvals/{id}", approvalV14)
	root.Handle("POST /v1/approvals/{id}/decision", approvalV14)

	// Scoped platform API: legacy admin secret remains accepted, while durable
	// access keys are evaluated against the path-level read/write/automation/admin
	// permission boundary before reaching the v1.4 control-plane handler.
	platformScoped := pm.GovernedScopedAuthHandler(cfg.Server.AdminTokenEnv, pm.HandlerV14())
	identityAdmin := pm.AdminAuthHandler(cfg.Server.AdminTokenEnv, pm.IdentityHandler())
	inboundAdmin := pm.AdminAuthHandler(cfg.Server.AdminTokenEnv, pm.InboundAdminHandler())
	statusScoped := pm.ScopedAuthHandler(cfg.Server.AdminTokenEnv, pm.StatusHandler())
	root.Handle("/v1/platform/identities", identityAdmin)
	root.Handle("/v1/platform/identities/", identityAdmin)
	root.Handle("/v1/platform/access-keys", identityAdmin)
	root.Handle("/v1/platform/access-keys/", identityAdmin)
	root.Handle("/v1/platform/inbound-receipts", inboundAdmin)
	root.Handle("/v1/platform/inbound-receipts/", inboundAdmin)
	root.Handle("/v1/platform/status", statusScoped)
	root.Handle("/v1/platform/metrics", statusScoped)
	root.Handle("/v1/platform/", platformScoped)

	// Scoped readers can see only approved knowledge. Proposal creation,
	// inspection and review live under the distinct admin namespace.
	knowledgeRead := pm.ScopedAuthHandler(cfg.Server.AdminTokenEnv, ks.ReadHandler())
	knowledgeAdmin := pm.AdminAuthHandler(cfg.Server.AdminTokenEnv, ks.AdminHandler())
	root.Handle("GET /v1/knowledge/items", knowledgeRead)
	root.Handle("GET /v1/knowledge/items/", knowledgeRead)
	root.Handle("GET /v1/knowledge/search", knowledgeRead)
	root.Handle("GET /v1/knowledge/neighbors", knowledgeRead)
	root.Handle("/v1/knowledge/admin/", knowledgeAdmin)

	// Cluster administration is privileged. Workers never receive admin tokens;
	// they authenticate with one-time-issued worker credentials on a separate API.
	clusterAdmin := pm.AdminAuthHandler(cfg.Server.AdminTokenEnv, cc.AdminHandler())
	root.Handle("/v1/cluster/workers", clusterAdmin)
	root.Handle("/v1/cluster/workers/", clusterAdmin)
	root.Handle("/v1/cluster/jobs", clusterAdmin)
	root.Handle("/v1/cluster/jobs/", clusterAdmin)
	root.Handle("/v1/cluster/worker/", cc.WorkerHandler())

	// Cognitive runtime provides durable operational continuity, learned
	// experience, reflection and an inspectable self-model. It is deliberately
	// admin-only and is not a claim of subjective consciousness.
	cognitionAdmin := pm.AdminAuthHandler(cfg.Server.AdminTokenEnv, ce.Handler())
	root.Handle("/v1/cognition/", cognitionAdmin)

	// Evolution control can create proposals, record evaluations and progress a
	// reviewed artifact through stage/release/rollback. It never edits source or
	// deploys itself; all trust transitions require admin authority and audit.
	evolutionAdmin := pm.AdminAuthHandler(cfg.Server.AdminTokenEnv, ec.Handler())
	root.Handle("/v1/evolution/control/", evolutionAdmin)

	// WorkGraph is durable execution intent. Creation and every state transition
	// are admin-controlled here; model-facing graph proposal tools can be added
	// separately without giving the model approval or completion authority.
	workgraphAdmin := pm.AdminAuthHandler(cfg.Server.AdminTokenEnv, wgs.Handler())
	root.Handle("/v1/workgraphs", workgraphAdmin)
	root.Handle("/v1/workgraphs/", workgraphAdmin)

	// Capability Envelopes are immutable authority grants. Delegation can only
	// narrow authority, and revoking a parent makes every descendant ineffective.
	// Budget usage/preflight stays on the same admin-only authority surface;
	// preflight is advisory and execution always rechecks atomically.
	authorityAdmin := pm.AdminAuthHandler(cfg.Server.AdminTokenEnv, authorityStore.Handler())
	root.Handle("/v1/authority/envelopes", authorityAdmin)
	root.Handle("/v1/authority/envelopes/", authorityAdmin)
	root.Handle("/v1/authority/usage", authorityAdmin)

	// Orchestration performs the race-free handoff from a Ready/approved
	// WorkGraph execute/delegate node to an authority-bound Cluster job. This
	// surface is admin-only; the model cannot dispatch, approve or reconcile it.
	orchestrationAdmin := pm.AdminAuthHandler(cfg.Server.AdminTokenEnv, orchestrator.Handler())
	root.Handle("/v1/orchestration/", orchestrationAdmin)

	// Channel-specific inbound authentication, durable Session submission and
	// conservative retry/reconciliation semantics are enforced in the v1.4 gateway.
	root.Handle("/v1/inbound/", pm.InboundHandlerV14())
	root.Handle("/", coreHandler)

	srv := &http.Server{Addr: cfg.Server.Listen, Handler: root, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: time.Duration(cfg.Runtime.RequestTimeoutSeconds+15) * time.Second, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 32 << 10}
	go func() {
		slog.Info("KINGAIBOT started", "listen", cfg.Server.Listen, "version", version, "platform", "v1.4-safe", "knowledge", "safe", "cognition", "enabled", "cluster", "enabled", "evolution_control", "enabled", "workgraph", "enabled", "authority", "enabled", "orchestration", "enabled")
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

func must(err error) {
	if err != nil {
		slog.Error("startup failed", "error", err)
		os.Exit(1)
	}
}
