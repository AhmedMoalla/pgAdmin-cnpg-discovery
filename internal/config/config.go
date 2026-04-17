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
	PgAdminURL      string
	PgAdminEmail    string
	PgAdminPassword string
	ServerGroupName string
	Namespace       string // empty = all namespaces
}

func Load() (*Config, error) {
	pollStr := getEnv("POLL_INTERVAL", "30s")
	poll, err := time.ParseDuration(pollStr)
	if err != nil {
		return nil, fmt.Errorf("invalid POLL_INTERVAL %q: %w", pollStr, err)
	}

	email := os.Getenv("PGADMIN_DEFAULT_EMAIL")
	if email == "" {
		return nil, fmt.Errorf("PGADMIN_DEFAULT_EMAIL is required")
	}
	password := os.Getenv("PGADMIN_DEFAULT_PASSWORD")
	if password == "" {
		return nil, fmt.Errorf("PGADMIN_DEFAULT_PASSWORD is required")
	}

	return &Config{
		PollInterval:    poll,
		ServersJSONPath: getEnv("SERVERS_JSON_PATH", "/shared/servers.json"),
		PgpassPath:      getEnv("PGPASS_PATH", "/shared/.pgpass"),
		PgAdminURL:      getEnv("PGADMIN_URL", "http://localhost:80"),
		PgAdminEmail:    email,
		PgAdminPassword: password,
		ServerGroupName: getEnv("SERVER_GROUP_NAME", "CNPG Clusters"),
		Namespace:       os.Getenv("NAMESPACE"),
	}, nil
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
