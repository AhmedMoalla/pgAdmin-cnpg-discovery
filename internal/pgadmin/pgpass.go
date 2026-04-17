package pgadmin

import (
	"fmt"
	"strings"

	"github.com/AhmedMoalla/pgadmin-cnpg-discovery/internal/discovery"
)

// GeneratePgpass creates .pgpass file content from discovered clusters.
// Format: hostname:port:database:username:password
// Special characters (:, \) in fields are escaped with \.
func GeneratePgpass(clusters []discovery.ClusterInfo) string {
	var b strings.Builder
	sorted := SortClusters(clusters)
	for _, c := range sorted {
		fmt.Fprintf(&b, "%s:%s:%s:%s:%s\n",
			escapePgpass(c.Host),
			escapePgpass(c.Port),
			escapePgpass(c.Database),
			escapePgpass(c.Username),
			escapePgpass(c.Password),
		)
	}
	return b.String()
}

// WritePgpass writes the .pgpass file atomically with mode 0600.
func WritePgpass(path string, clusters []discovery.ClusterInfo) error {
	data := GeneratePgpass(clusters)
	return atomicWrite(path, []byte(data), 0600)
}

// escapePgpass escapes special characters in .pgpass fields.
func escapePgpass(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `:`, `\:`)
	return s
}
