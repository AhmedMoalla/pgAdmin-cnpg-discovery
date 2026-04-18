package reconciler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/AhmedMoalla/pgadmin-cnpg-discovery/internal/config"
	"github.com/AhmedMoalla/pgadmin-cnpg-discovery/internal/discovery"
	"github.com/AhmedMoalla/pgadmin-cnpg-discovery/internal/pgadmin"
)

// Reconciler periodically discovers CNPG clusters and syncs them to pgAdmin.
type Reconciler struct {
	cfg        *config.Config
	discoverer *discovery.Discoverer

	lastAppliedConfigHash string
	hasAppliedConfig      bool
}

// New creates a new Reconciler.
func New(cfg *config.Config, discoverer *discovery.Discoverer) *Reconciler {
	return &Reconciler{
		cfg:        cfg,
		discoverer: discoverer,
	}
}

// Run starts the reconciliation loop. It blocks until the context is cancelled.
func (r *Reconciler) Run(ctx context.Context) error {
	slog.Info("starting reconciler", "interval", r.cfg.PollInterval)

	// Run immediately on start
	r.reconcile(ctx)

	ticker := time.NewTicker(r.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("reconciler stopped")
			return ctx.Err()
		case <-ticker.C:
			r.reconcile(ctx)
		}
	}
}

// reconcile performs one full discovery + file generation cycle.
func (r *Reconciler) reconcile(ctx context.Context) {
	slog.Debug("starting reconciliation cycle")

	clusters, err := r.discoverer.DiscoverClusters(ctx)
	if err != nil {
		slog.Error("failed to discover clusters", "error", err)
		return
	}

	configHash, err := r.configHash(clusters)
	if err != nil {
		slog.Error("failed to hash discovered configuration", "error", err)
		return
	}

	if r.hasAppliedConfig && configHash == r.lastAppliedConfigHash {
		slog.Debug("configuration unchanged; skipping update", "count", len(clusters))
		return
	}

	slog.Info("discovered clusters", "count", len(clusters))

	wasAlreadyApplied := r.hasAppliedConfig
	if r.writeFiles(clusters) {
		r.lastAppliedConfigHash = configHash
		r.hasAppliedConfig = true
		if wasAlreadyApplied {
			r.restartPod(ctx)
		}
	}
}

// writeFiles writes servers.json and .pgpass to the shared volume.
func (r *Reconciler) writeFiles(clusters []discovery.ClusterInfo) bool {
	success := true

	if err := pgadmin.WriteServersJSON(r.cfg.ServersJSONPath, clusters, r.cfg.ServerGroupName); err != nil {
		slog.Error("failed to write servers.json", "path", r.cfg.ServersJSONPath, "error", err)
		success = false
	} else {
		slog.Debug("wrote servers.json", "path", r.cfg.ServersJSONPath, "servers", len(clusters))
	}

	if err := pgadmin.WritePgpass(r.cfg.PgpassPath, clusters); err != nil {
		slog.Error("failed to write .pgpass", "path", r.cfg.PgpassPath, "error", err)
		success = false
	} else {
		slog.Debug("wrote .pgpass", "path", r.cfg.PgpassPath, "entries", len(clusters))
	}

	return success
}

// restartPod deletes the sidecar's own pod, causing Kubernetes to restart both
// the sidecar and the pgAdmin container so pgAdmin re-reads servers.json.
func (r *Reconciler) restartPod(ctx context.Context) {
	if r.cfg.PodName == "" || r.cfg.PodNamespace == "" {
		slog.Warn("skipping pod restart: POD_NAME or POD_NAMESPACE not configured")
		return
	}
	slog.Info("restarting pod to reload pgAdmin servers.json",
		"pod", r.cfg.PodName, "namespace", r.cfg.PodNamespace)
	err := r.discoverer.Clientset().CoreV1().Pods(r.cfg.PodNamespace).Delete(
		ctx, r.cfg.PodName, metav1.DeleteOptions{})
	if err != nil {
		slog.Error("failed to delete pod for restart", "pod", r.cfg.PodName, "error", err)
	}
}

func (r *Reconciler) configHash(clusters []discovery.ClusterInfo) (string, error) {
	payload := struct {
		ServerGroupName string                  `json:"server_group_name"`
		Clusters        []discovery.ClusterInfo `json:"clusters"`
	}{
		ServerGroupName: r.cfg.ServerGroupName,
		Clusters:        pgadmin.SortClusters(clusters),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshaling configuration payload: %w", err)
	}

	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// ReconcileOnce runs a single reconciliation cycle (useful for testing).
func (r *Reconciler) ReconcileOnce(ctx context.Context) error {
	clusters, err := r.discoverer.DiscoverClusters(ctx)
	if err != nil {
		return fmt.Errorf("discovering clusters: %w", err)
	}
	r.writeFiles(clusters)
	return nil
}
