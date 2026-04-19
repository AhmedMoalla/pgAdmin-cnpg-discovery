package discovery

// ClusterInfo holds the connection details for a discovered CNPG cluster.
type ClusterInfo struct {
	Name      string
	Namespace string
	Host      string
	Port      string
	Username  string
	Password  string
	Database  string
	Pgpass    string
}

// ServerKey returns a unique identifier for this cluster entry.
func (c ClusterInfo) ServerKey() string {
	return c.Namespace + "/" + c.Name
}
