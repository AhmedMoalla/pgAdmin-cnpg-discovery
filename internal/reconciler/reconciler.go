package reconciler

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/AhmedMoalla/pgadmin-cnpg-discovery/internal/config"
	"github.com/AhmedMoalla/pgadmin-cnpg-discovery/internal/discovery"
	"github.com/AhmedMoalla/pgadmin-cnpg-discovery/internal/pgadmin"
)

// Reconciler periodically discovers CNPG clusters and syncs them to pgAdmin.
type Reconciler struct {
	cfg        *config.Config
	discoverer *discovery.Discoverer
	apiClient  *pgadmin.APIClient
}

// New creates a new Reconciler.
func New(cfg *config.Config, discoverer *discovery.Discoverer) *Reconciler {
	return &Reconciler{
		cfg:        cfg,
		discoverer: discoverer,
		apiClient:  pgadmin.NewAPIClient(cfg.PgAdminURL, cfg.PgAdminEmail, cfg.PgAdminPassword),
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

// reconcile performs one full discovery + sync cycle.
func (r *Reconciler) reconcile(ctx context.Context) {
	slog.Debug("starting reconciliation cycle")

	clusters, err := r.discoverer.DiscoverClusters(ctx)
	if err != nil {
		slog.Error("failed to discover clusters", "error", err)
		return
	}
	slog.Info("discovered clusters", "count", len(clusters))

	// Phase 1: Always write files (for startup/restart scenarios)
	r.writeFiles(clusters)

	// Phase 2: Try live sync via pgAdmin API
	r.syncViaAPI(clusters)
}

// writeFiles writes servers.json and .pgpass to the shared volume.
func (r *Reconciler) writeFiles(clusters []discovery.ClusterInfo) {
	if err := pgadmin.WriteServersJSON(r.cfg.ServersJSONPath, clusters, r.cfg.ServerGroupName); err != nil {
		slog.Error("failed to write servers.json", "path", r.cfg.ServersJSONPath, "error", err)
	} else {
		slog.Debug("wrote servers.json", "path", r.cfg.ServersJSONPath, "servers", len(clusters))
	}

	if err := pgadmin.WritePgpass(r.cfg.PgpassPath, clusters); err != nil {
		slog.Error("failed to write .pgpass", "path", r.cfg.PgpassPath, "error", err)
	} else {
		slog.Debug("wrote .pgpass", "path", r.cfg.PgpassPath, "entries", len(clusters))
	}
}

// syncViaAPI syncs the discovered clusters with pgAdmin's REST API.
func (r *Reconciler) syncViaAPI(clusters []discovery.ClusterInfo) {
	if !r.apiClient.IsAvailable() {
		slog.Warn("pgAdmin not available, skipping API sync")
		return
	}

	groupID, err := r.apiClient.FindOrCreateServerGroup(r.cfg.ServerGroupName)
	if err != nil {
		slog.Error("failed to find/create server group", "error", err)
		return
	}

	currentServers, err := r.apiClient.ListServers(groupID)
	if err != nil {
		slog.Error("failed to list current servers", "error", err)
		return
	}

	// Build sets for diffing
	desiredByKey := make(map[string]discovery.ClusterInfo)
	for _, c := range clusters {
		desiredByKey[c.ServerKey()] = c
	}

	// Find managed servers in current (only those with our comment)
	managedCurrent := make(map[string]pgadmin.APIServer)
	for _, s := range currentServers {
		if s.Comment == "Managed by cnpg-discovery" {
			managedCurrent[s.Name] = s
		}
	}

	// Add new servers (desired but not in managed current)
	for key, cluster := range desiredByKey {
		if _, exists := managedCurrent[key]; exists {
			continue
		}
		port, _ := strconv.Atoi(cluster.Port)
		if port == 0 {
			port = 5432
		}
		entry := pgadmin.ServerEntry{
			Name:          cluster.ServerKey(),
			Group:         r.cfg.ServerGroupName,
			Host:          cluster.Host,
			Port:          port,
			MaintenanceDB: cluster.Database,
			Username:      cluster.Username,
			SSLMode:       "prefer",
			Comment:       "Managed by cnpg-discovery",
		}
		if err := r.apiClient.CreateServer(groupID, entry); err != nil {
			slog.Error("failed to create server", "name", key, "error", err)
		}
	}

	// Remove stale servers (managed current but not in desired)
	for key, server := range managedCurrent {
		if _, exists := desiredByKey[key]; exists {
			continue
		}
		if err := r.apiClient.DeleteServer(groupID, server.ID); err != nil {
			slog.Error("failed to delete server", "name", key, "id", server.ID, "error", err)
		}
	}

	slog.Debug("API sync complete",
		"desired", len(desiredByKey),
		"current_managed", len(managedCurrent),
	)
}

// ReconcileOnce runs a single reconciliation cycle (useful for testing).
func (r *Reconciler) ReconcileOnce(ctx context.Context) error {
	clusters, err := r.discoverer.DiscoverClusters(ctx)
	if err != nil {
		return fmt.Errorf("discovering clusters: %w", err)
	}
	r.writeFiles(clusters)
	r.syncViaAPI(clusters)
	return nil
}
