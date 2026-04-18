package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	PollInterval    time.Duration
	ServersJSONPath string
	PgpassPath      string
	ServerGroupName string
	Namespace       string // empty = all namespaces
	PodName         string // set via POD_NAME downward API env var
	PodNamespace    string // set via POD_NAMESPACE downward API env var
}

func Load() (*Config, error) {
	pollStr := getEnv("POLL_INTERVAL", "30s")
	poll, err := time.ParseDuration(pollStr)
	if err != nil {
		return nil, fmt.Errorf("invalid POLL_INTERVAL %q: %w", pollStr, err)
	}

	return &Config{
		PollInterval:    poll,
		ServersJSONPath: getEnv("SERVERS_JSON_PATH", "/shared/servers.json"),
		PgpassPath:      getEnv("PGPASS_PATH", "/shared/.pgpass"),
		ServerGroupName: getEnv("SERVER_GROUP_NAME", "CNPG Clusters"),
		Namespace:       os.Getenv("NAMESPACE"),
		PodName:         getEnv("POD_NAME", os.Getenv("HOSTNAME")),
		PodNamespace:    os.Getenv("POD_NAMESPACE"),
	}, nil
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
