package pgadmin

import (
	"os"
	"testing"

	"github.com/AhmedMoalla/pgadmin-cnpg-discovery/internal/discovery"
)

func TestGeneratePgpass(t *testing.T) {
	tests := []struct {
		name     string
		clusters []discovery.ClusterInfo
		want     string
	}{
		{
			name:     "empty clusters",
			clusters: []discovery.ClusterInfo{},
			want:     "",
		},
		{
			name: "single cluster",
			clusters: []discovery.ClusterInfo{
				{
					Name:      "test-cluster",
					Namespace: "default",
					Pgpass:    "localhost:5432:postgres:postgres:password",
				},
			},
			want: "localhost:5432:postgres:postgres:password\n",
		},
		{
			name: "multiple clusters sorted by ServerKey",
			clusters: []discovery.ClusterInfo{
				{
					Name:      "z-cluster",
					Namespace: "b-ns",
					Pgpass:    "z-host:5432:z-db:z-user:z-pass",
				},
				{
					Name:      "a-cluster",
					Namespace: "a-ns",
					Pgpass:    "a-host:5432:a-db:a-user:a-pass",
				},
			},
			want: "a-host:5432:a-db:a-user:a-pass\nz-host:5432:z-db:z-user:z-pass\n",
		},
		{
			name: "retains special characters from secret pgpass",
			clusters: []discovery.ClusterInfo{
				{
					Name:      "test",
					Namespace: "default",
					Pgpass:    "host\\:with\\:colons:5432:db:user\\\\with\\\\backslashes:pass\\:with\\:colons\\:and\\\\backslashes",
				},
			},
			want: "host\\:with\\:colons:5432:db:user\\\\with\\\\backslashes:pass\\:with\\:colons\\:and\\\\backslashes\n",
		},
		{
			name: "trims surrounding whitespace and ensures single newline",
			clusters: []discovery.ClusterInfo{
				{
					Name:      "test",
					Namespace: "default",
					Pgpass:    "  localhost:5432:postgres:postgres:password\n\t",
				},
			},
			want: "localhost:5432:postgres:postgres:password\n",
		},
		{
			name: "skips clusters with empty pgpass",
			clusters: []discovery.ClusterInfo{
				{
					Name:      "has-pgpass",
					Namespace: "default",
					Pgpass:    "localhost:5432:db:user:pass",
				},
				{
					Name:      "no-pgpass",
					Namespace: "default",
					Pgpass:    "",
				},
			},
			want: "localhost:5432:db:user:pass\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GeneratePgpass(tt.clusters)
			if got != tt.want {
				t.Errorf("GeneratePgpass() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWritePgpass(t *testing.T) {
	t.Run("writes file with 0600 permissions", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := tmpDir + "/.pgpass"

		clusters := []discovery.ClusterInfo{
			{
				Name:      "test",
				Namespace: "default",
				Pgpass:    "localhost:5432:postgres:postgres:password",
			},
		}

		err := WritePgpass(path, clusters)
		if err != nil {
			t.Fatalf("WritePgpass() error = %v", err)
		}

		// Check file exists
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat failed: %v", err)
		}

		// Check permissions are 0600
		if info.Mode()&0777 != 0600 {
			t.Errorf("file permissions = %o, want 0600", info.Mode()&0777)
		}

		// Check content
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}

		expected := "localhost:5432:postgres:postgres:password\n"
		if string(content) != expected {
			t.Errorf("file content = %q, want %q", string(content), expected)
		}
	})

	t.Run("atomic write on nonexistent path", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := tmpDir + "/subdir/.pgpass"

		clusters := []discovery.ClusterInfo{
			{
				Name:      "test",
				Namespace: "default",
				Pgpass:    "localhost:5432:db:user:pass",
			},
		}

		err := WritePgpass(path, clusters)
		if err != nil {
			t.Fatalf("WritePgpass() error = %v", err)
		}

		// File should be created along with parent directories
		if _, err := os.Stat(path); err != nil {
			t.Errorf("file not created: %v", err)
		}
	})

	t.Run("overwrites existing file", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := tmpDir + "/.pgpass"

		// Write initial content
		err := os.WriteFile(path, []byte("old content"), 0644)
		if err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}

		clusters := []discovery.ClusterInfo{
			{
				Name:      "new",
				Namespace: "default",
				Pgpass:    "newhost:5433:newdb:newuser:newpass",
			},
		}

		err = WritePgpass(path, clusters)
		if err != nil {
			t.Fatalf("WritePgpass() error = %v", err)
		}

		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}

		expected := "newhost:5433:newdb:newuser:newpass\n"
		if string(content) != expected {
			t.Errorf("file content = %q, want %q", string(content), expected)
		}
	})
}
