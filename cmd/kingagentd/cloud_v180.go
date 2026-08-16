package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kingaiwork/KINGAIBOT/internal/cloud"
	"github.com/kingaiwork/KINGAIBOT/internal/cognition"
	"github.com/kingaiwork/KINGAIBOT/internal/config"
	"github.com/kingaiwork/KINGAIBOT/internal/memory"
	"github.com/kingaiwork/KINGAIBOT/internal/platform"
)

func envBool(name string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	if value == "" {
		return fallback
	}
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func envSeconds(name string, fallback int) time.Duration {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(name)))
	if err != nil || value <= 0 {
		value = fallback
	}
	return time.Duration(value) * time.Second
}

func prepareCloud(cfg *config.Config, agentVersion string) (*cloud.Manager, cloud.Policy, error) {
	enrollmentPresent := strings.TrimSpace(os.Getenv("KINGAI_ENROLLMENT_TOKEN")) != ""
	enabled := envBool("KINGAI_CLOUD_ENABLED", enrollmentPresent)
	manager, err := cloud.New(filepath.Join(cfg.Runtime.DataDir, "cloud"), cloud.Config{
		Enabled:             enabled,
		BaseURL:             strings.TrimSpace(os.Getenv("KINGAI_CLOUD_BASE_URL")),
		EnrollmentTokenEnv:  "KINGAI_ENROLLMENT_TOKEN",
		Environment:         strings.TrimSpace(os.Getenv("KINGAI_CLOUD_ENVIRONMENT")),
		NodeClass:           strings.TrimSpace(os.Getenv("KINGAI_NODE_CLASS")),
		Provider:            strings.TrimSpace(os.Getenv("KINGAI_NODE_PROVIDER")),
		Region:               strings.TrimSpace(os.Getenv("KINGAI_NODE_REGION")),
		HeartbeatInterval:   envSeconds("KINGAI_CLOUD_HEARTBEAT_SECONDS", 60),
		SyncInterval:        envSeconds("KINGAI_MEMORY_SYNC_SECONDS", 900),
		SyncEnabled:         envBool("KINGAI_MEMORY_SYNC", false),
		SyncKeyEnv:          "KINGAI_SYNC_KEY",
		AllowCustomEndpoint: envBool("KINGAI_CLOUD_ALLOW_CUSTOM_ENDPOINT", false),
	})
	if err != nil {
		return nil, cloud.Policy{}, err
	}
	if !enabled {
		return manager, cloud.Policy{}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	policy, bootstrapErr := manager.Bootstrap(ctx, agentVersion)
	if bootstrapErr != nil && envBool("KINGAI_CLOUD_REQUIRE_POLICY", false) {
		manager.Close()
		return nil, cloud.Policy{}, bootstrapErr
	}
	cloud.ApplyRestrictions(cfg, policy)
	return manager, policy, nil
}

type encryptedContinuitySnapshot struct {
	SchemaVersion int                `json:"schema_version"`
	GeneratedAt   time.Time          `json:"generated_at"`
	Memory        []memory.Record    `json:"memory"`
	Cognition     cognition.Snapshot `json:"cognition"`
	Notice        string             `json:"notice"`
}

func startCloud(manager *cloud.Manager, cfg *config.Config, pm *platform.Manager, ms *memory.Store, ce *cognition.Engine, startedAt time.Time) {
	if manager == nil {
		return
	}
	manager.Start(func() cloud.Telemetry {
		health := 100
		status := "healthy"
		if snapshot, err := pm.StatusSnapshot(); err != nil || snapshot.AttentionRequired {
			health, status = 70, "warning"
		}
		return cloud.Telemetry{Status: status, HealthScore: health, Uptime: time.Since(startedAt), AgentVersion: cfg.Version}
	}, func(ctx context.Context) ([]byte, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		records, err := ms.SnapshotRecords(512)
		if err != nil {
			return nil, err
		}
		snapshot := encryptedContinuitySnapshot{SchemaVersion: 1, GeneratedAt: time.Now().UTC(), Memory: records, Cognition: ce.Snapshot(), Notice: "E2EE continuity snapshot. Cognitive state is preserved for recovery/inspection but is not automatically merged into another node's self-model."}
		return json.Marshal(snapshot)
	}, func(ctx context.Context, payload []byte) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		var snapshot encryptedContinuitySnapshot
		if err := json.Unmarshal(payload, &snapshot); err != nil {
			return err
		}
		if snapshot.SchemaVersion != 1 || len(snapshot.Memory) > 2048 {
			return errors.New("unsupported encrypted continuity snapshot")
		}
		_, err := ms.MergeSyncedRecords(snapshot.Memory)
		return err
	})
}
