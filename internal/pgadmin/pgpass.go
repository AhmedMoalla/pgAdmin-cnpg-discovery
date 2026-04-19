package pgadmin

import (
	"strings"

	"github.com/AhmedMoalla/pgadmin-cnpg-discovery/internal/discovery"
)

// GeneratePgpass creates .pgpass file content from discovered clusters.
// Entries come directly from the CNPG secret's "pgpass" key.
func GeneratePgpass(clusters []discovery.ClusterInfo) string {
	var b strings.Builder
	sorted := SortClusters(clusters)
	for _, c := range sorted {
		entry := strings.TrimSpace(c.Pgpass)
		if entry == "" {
			continue
		}
		b.WriteString(entry)
		b.WriteByte('\n')
	}
	return b.String()
}

// WritePgpass writes the .pgpass file atomically with mode 0600.
func WritePgpass(path string, clusters []discovery.ClusterInfo) error {
	data := GeneratePgpass(clusters)
	return atomicWrite(path, []byte(data), 0600)
}
