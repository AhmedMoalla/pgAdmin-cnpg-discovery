package config

import (
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    *Config
		wantErr bool
	}{
		{
			name: "defaults",
			env:  map[string]string{},
			want: &Config{
				PollInterval:    30 * time.Second,
				ServersJSONPath: "/shared/servers.json",
				PgpassPath:      "/shared/.pgpass",
				ServerGroupName: "CNPG Clusters",
				Namespace:       "",
			},
			wantErr: false,
		},
		{
			name: "all vars customized",
			env: map[string]string{
				"POLL_INTERVAL":     "1m",
				"SERVERS_JSON_PATH": "/custom/servers.json",
				"PGPASS_PATH":       "/custom/.pgpass",
				"SERVER_GROUP_NAME": "My Clusters",
				"NAMESPACE":         "production",
			},
			want: &Config{
				PollInterval:    60 * time.Second,
				ServersJSONPath: "/custom/servers.json",
				PgpassPath:      "/custom/.pgpass",
				ServerGroupName: "My Clusters",
				Namespace:       "production",
			},
			wantErr: false,
		},
		{
			name: "invalid POLL_INTERVAL",
			env: map[string]string{
				"POLL_INTERVAL": "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			for key, val := range tt.env {
				t.Setenv(key, val)
			}
			// Clear unset vars
			for _, key := range []string{"POLL_INTERVAL", "SERVERS_JSON_PATH", "PGPASS_PATH", "SERVER_GROUP_NAME", "NAMESPACE"} {
				if _, ok := tt.env[key]; !ok {
					t.Setenv(key, "")
				}
			}

			got, err := Load()
			if (err != nil) != tt.wantErr {
				t.Errorf("Load() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}

			if got.PollInterval != tt.want.PollInterval {
				t.Errorf("PollInterval = %v, want %v", got.PollInterval, tt.want.PollInterval)
			}
			if got.ServersJSONPath != tt.want.ServersJSONPath {
				t.Errorf("ServersJSONPath = %q, want %q", got.ServersJSONPath, tt.want.ServersJSONPath)
			}
			if got.PgpassPath != tt.want.PgpassPath {
				t.Errorf("PgpassPath = %q, want %q", got.PgpassPath, tt.want.PgpassPath)
			}
			if got.ServerGroupName != tt.want.ServerGroupName {
				t.Errorf("ServerGroupName = %q, want %q", got.ServerGroupName, tt.want.ServerGroupName)
			}
			if got.Namespace != tt.want.Namespace {
				t.Errorf("Namespace = %q, want %q", got.Namespace, tt.want.Namespace)
			}
		})
	}
}

func TestGetEnv(t *testing.T) {
	tests := []struct {
		name       string
		key        string
		defaultVal string
		envVal     string
		want       string
	}{
		{
			name:       "env var set",
			key:        "TEST_VAR",
			envVal:     "custom",
			defaultVal: "default",
			want:       "custom",
		},
		{
			name:       "env var empty string",
			key:        "TEST_VAR",
			envVal:     "",
			defaultVal: "default",
			want:       "default",
		},
		{
			name:       "env var not set returns default",
			key:        "NONEXISTENT_VAR",
			defaultVal: "my_default",
			want:       "my_default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envVal != "" || tt.name == "env var empty string" {
				t.Setenv(tt.key, tt.envVal)
			}

			got := getEnv(tt.key, tt.defaultVal)
			if got != tt.want {
				t.Errorf("getEnv(%q, %q) = %q, want %q", tt.key, tt.defaultVal, got, tt.want)
			}
		})
	}
}
