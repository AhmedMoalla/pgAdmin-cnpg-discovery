package pgadmin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/AhmedMoalla/pgadmin-cnpg-discovery/internal/discovery"
)

// ServerEntry represents a single server in pgAdmin's servers.json format.
type ServerEntry struct {
	Name                 string         `json:"Name"`
	Group                string         `json:"Group"`
	Host                 string         `json:"Host"`
	Port                 int            `json:"Port"`
	MaintenanceDB        string         `json:"MaintenanceDB"`
	Username             string         `json:"Username"`
	Comment              string         `json:"Comment,omitempty"`
	ConnectionParameters map[string]any `json:"ConnectionParameters,omitempty"`
}

// ServersJSON is the top-level structure of pgAdmin's servers.json file.
type ServersJSON struct {
	Servers map[string]ServerEntry `json:"Servers"`
}

const managedComment = "Managed by cnpg-discovery"
const pgAdminConnectionPassfile = "/.pgpass"

// GenerateServersJSON creates the servers.json content from discovered clusters.
func GenerateServersJSON(clusters []ClusterInfoSorted, groupName, _ string) ([]byte, error) {
	servers := ServersJSON{
		Servers: make(map[string]ServerEntry),
	}

	for i, c := range clusters {
		port, err := strconv.Atoi(c.Port)
		if err != nil {
			port = 5432
		}

		id := strconv.Itoa(i + 1)
		servers.Servers[id] = ServerEntry{
			Name:          c.ServerKey(),
			Group:         groupName,
			Host:          c.Host,
			Port:          port,
			MaintenanceDB: c.Database,
			Username:      c.Username,
			Comment:       managedComment,
			ConnectionParameters: map[string]any{
				"sslmode":         "prefer",
				"connect_timeout": 10,
				"passfile":        pgAdminConnectionPassfile,
			},
		}
	}

	return json.MarshalIndent(servers, "", "  ")
}

// ClusterInfoSorted is a type alias to allow deterministic sorting of clusters.
type ClusterInfoSorted = discovery.ClusterInfo

// SortClusters returns a sorted copy of clusters for deterministic ID assignment.
func SortClusters(clusters []discovery.ClusterInfo) []ClusterInfoSorted {
	sorted := make([]ClusterInfoSorted, len(clusters))
	copy(sorted, clusters)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ServerKey() < sorted[j].ServerKey()
	})
	return sorted
}

// WriteServersJSON writes the servers.json file atomically.
func WriteServersJSON(path string, clusters []discovery.ClusterInfo, groupName, pgpassPath string) error {
	sorted := SortClusters(clusters)
	data, err := GenerateServersJSON(sorted, groupName, pgpassPath)
	if err != nil {
		return fmt.Errorf("generating servers.json: %w", err)
	}
	return atomicWrite(path, data, 0644)
}

// atomicWrite writes data to a temp file and renames it to the target path.
func atomicWrite(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("setting permissions: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("renaming temp file: %w", err)
	}
	return nil
}
