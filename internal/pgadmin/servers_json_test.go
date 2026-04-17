package pgadmin

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/AhmedMoalla/pgadmin-cnpg-discovery/internal/discovery"
)

func TestSortClusters(t *testing.T) {
	tests := []struct {
		name             string
		input            []discovery.ClusterInfo
		wantFirstKey     string
		wantLastKey      string
		wantLen          int
	}{
		{
			name:        "empty list",
			input:       []discovery.ClusterInfo{},
			wantLen:     0,
		},
		{
			name: "already sorted",
			input: []discovery.ClusterInfo{
				{Name: "a", Namespace: "ns1"},
				{Name: "b", Namespace: "ns1"},
			},
			wantFirstKey: "ns1/a",
			wantLastKey:  "ns1/b",
			wantLen:      2,
		},
		{
			name: "reverse sorted",
			input: []discovery.ClusterInfo{
				{Name: "z", Namespace: "ns2"},
				{Name: "a", Namespace: "ns1"},
			},
			wantFirstKey: "ns1/a",
			wantLastKey:  "ns2/z",
			wantLen:      2,
		},
		{
			name: "namespace then name sort",
			input: []discovery.ClusterInfo{
				{Name: "z", Namespace: "prod"},
				{Name: "a", Namespace: "dev"},
				{Name: "m", Namespace: "prod"},
			},
			wantFirstKey: "dev/a",
			wantLastKey:  "prod/z",
			wantLen:      3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SortClusters(tt.input)
			
			if len(got) != tt.wantLen {
				t.Errorf("SortClusters() len = %d, want %d", len(got), tt.wantLen)
			}
			
			if tt.wantLen > 0 {
				if got[0].ServerKey() != tt.wantFirstKey {
					t.Errorf("first key = %q, want %q", got[0].ServerKey(), tt.wantFirstKey)
				}
				if got[len(got)-1].ServerKey() != tt.wantLastKey {
					t.Errorf("last key = %q, want %q", got[len(got)-1].ServerKey(), tt.wantLastKey)
				}
			}
		})
	}
}

func TestGenerateServersJSON(t *testing.T) {
	tests := []struct {
		name      string
		clusters  []discovery.ClusterInfo
		groupName string
		validate  func(*ServersJSON) error
	}{
		{
			name:      "empty clusters",
			clusters:  []discovery.ClusterInfo{},
			groupName: "CNPG Clusters",
			validate: func(s *ServersJSON) error {
				if len(s.Servers) != 0 {
					t.Errorf("Servers count = %d, want 0", len(s.Servers))
				}
				return nil
			},
		},
		{
			name: "single cluster",
			clusters: []discovery.ClusterInfo{
				{
					Name:      "test-cluster",
					Namespace: "default",
					Host:      "localhost",
					Port:      "5432",
					Username:  "postgres",
					Password:  "password",
					Database:  "postgres",
				},
			},
			groupName: "CNPG Clusters",
			validate: func(s *ServersJSON) error {
				if len(s.Servers) != 1 {
					t.Errorf("Servers count = %d, want 1", len(s.Servers))
				}
				server, ok := s.Servers["1"]
				if !ok {
					t.Errorf("Server ID 1 not found")
					return nil
				}
				if server.Name != "default/test-cluster" {
					t.Errorf("Name = %q, want %q", server.Name, "default/test-cluster")
				}
				if server.Host != "localhost" {
					t.Errorf("Host = %q, want %q", server.Host, "localhost")
				}
				if server.Port != 5432 {
					t.Errorf("Port = %d, want %d", server.Port, 5432)
				}
				if server.MaintenanceDB != "postgres" {
					t.Errorf("MaintenanceDB = %q, want %q", server.MaintenanceDB, "postgres")
				}
				if server.Username != "postgres" {
					t.Errorf("Username = %q, want %q", server.Username, "postgres")
				}
				if server.SSLMode != "prefer" {
					t.Errorf("SSLMode = %q, want %q", server.SSLMode, "prefer")
				}
				if server.Comment != "Managed by cnpg-discovery" {
					t.Errorf("Comment = %q, want %q", server.Comment, "Managed by cnpg-discovery")
				}
				return nil
			},
		},
		{
			name: "multiple clusters get sequential IDs",
			clusters: []discovery.ClusterInfo{
				{Name: "a", Namespace: "ns1", Host: "h1", Port: "5432", Username: "u1", Database: "d1"},
				{Name: "b", Namespace: "ns1", Host: "h2", Port: "5432", Username: "u2", Database: "d2"},
			},
			groupName: "My Group",
			validate: func(s *ServersJSON) error {
				if len(s.Servers) != 2 {
					t.Errorf("Servers count = %d, want 2", len(s.Servers))
				}
				for i := 1; i <= 2; i++ {
					id := string(rune('0') + rune(i))
					if _, ok := s.Servers[id]; !ok {
						t.Errorf("Server ID %s not found", id)
					}
				}
				return nil
			},
		},
		{
			name: "port conversion from string",
			clusters: []discovery.ClusterInfo{
				{Name: "test", Namespace: "default", Host: "host", Port: "5433", Username: "user", Database: "db"},
			},
			groupName: "Testing",
			validate: func(s *ServersJSON) error {
				server := s.Servers["1"]
				if server.Port != 5433 {
					t.Errorf("Port = %d, want %d", server.Port, 5433)
				}
				return nil
			},
		},
		{
			name: "invalid port defaults to 5432",
			clusters: []discovery.ClusterInfo{
				{Name: "test", Namespace: "default", Host: "host", Port: "invalid", Username: "user", Database: "db"},
			},
			groupName: "Testing",
			validate: func(s *ServersJSON) error {
				server := s.Servers["1"]
				if server.Port != 5432 {
					t.Errorf("Port = %d, want %d (default)", server.Port, 5432)
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sorted := SortClusters(tt.clusters)
			got, err := GenerateServersJSON(sorted, tt.groupName)
			if err != nil {
				t.Fatalf("GenerateServersJSON() error = %v", err)
			}

			var s ServersJSON
			err = json.Unmarshal(got, &s)
			if err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}

			tt.validate(&s)
		})
	}
}

func TestWriteServersJSON(t *testing.T) {
	t.Run("writes file with 0644 permissions", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := tmpDir + "/servers.json"

		clusters := []discovery.ClusterInfo{
			{
				Name:      "test",
				Namespace: "default",
				Host:      "localhost",
				Port:      "5432",
				Username:  "postgres",
				Password:  "password",
				Database:  "postgres",
			},
		}

		err := WriteServersJSON(path, clusters, "CNPG Clusters")
		if err != nil {
			t.Fatalf("WriteServersJSON() error = %v", err)
		}

		// Check file exists
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat failed: %v", err)
		}

		// Check permissions are 0644
		if info.Mode()&0777 != 0644 {
			t.Errorf("file permissions = %o, want 0644", info.Mode()&0777)
		}

		// Check content is valid JSON
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}

		var s ServersJSON
		err = json.Unmarshal(content, &s)
		if err != nil {
			t.Fatalf("json.Unmarshal failed: %v", err)
		}

		if len(s.Servers) != 1 {
			t.Errorf("Servers count = %d, want 1", len(s.Servers))
		}
	})

	t.Run("creates parent directories", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := tmpDir + "/nested/deep/servers.json"

		clusters := []discovery.ClusterInfo{
			{
				Name:      "test",
				Namespace: "default",
				Host:      "localhost",
				Port:      "5432",
				Username:  "postgres",
				Password:  "password",
				Database:  "postgres",
			},
		}

		err := WriteServersJSON(path, clusters, "CNPG Clusters")
		if err != nil {
			t.Fatalf("WriteServersJSON() error = %v", err)
		}

		if _, err := os.Stat(path); err != nil {
			t.Errorf("file not created: %v", err)
		}
	})

	t.Run("overwrites existing file", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := tmpDir + "/servers.json"

		// Write initial content
		err := os.WriteFile(path, []byte("old content"), 0644)
		if err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}

		clusters := []discovery.ClusterInfo{
			{
				Name:      "new",
				Namespace: "default",
				Host:      "newhost",
				Port:      "5433",
				Username:  "newuser",
				Database:  "newdb",
			},
		}

		err = WriteServersJSON(path, clusters, "CNPG Clusters")
		if err != nil {
			t.Fatalf("WriteServersJSON() error = %v", err)
		}

		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}

		var s ServersJSON
		err = json.Unmarshal(content, &s)
		if err != nil {
			t.Fatalf("json.Unmarshal failed: %v", err)
		}

		if len(s.Servers) != 1 {
			t.Errorf("Servers count = %d, want 1", len(s.Servers))
		}
		server := s.Servers["1"]
		if server.Host != "newhost" {
			t.Errorf("Host = %q, want %q", server.Host, "newhost")
		}
	})
}
