package reconciler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	fakeClientset "k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"

	"github.com/AhmedMoalla/pgadmin-cnpg-discovery/internal/config"
	"github.com/AhmedMoalla/pgadmin-cnpg-discovery/internal/discovery"
)

// Helper to create fake CNPG cluster
func createFakeCNPGCluster(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "postgresql.cnpg.io/v1",
			"kind":       "Cluster",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{},
		},
	}
}

// Helper to convert objects
func toRuntimeObjectSlice(clusters []*unstructured.Unstructured) []runtime.Object {
	result := make([]runtime.Object, len(clusters))
	for i, c := range clusters {
		result[i] = c
	}
	return result
}

func toRuntimeObjectSecretsSlice(secrets []*v1.Secret) []runtime.Object {
	result := make([]runtime.Object, len(secrets))
	for i, s := range secrets {
		result[i] = s
	}
	return result
}

func TestNew(t *testing.T) {
	cfg := &config.Config{
		PollInterval:    30 * time.Second,
		ServersJSONPath: "/tmp/servers.json",
		PgpassPath:      "/tmp/.pgpass",
		ServerGroupName: "CNPG",
		Namespace:       "",
	}

	// Create fake discoverer
	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "ClusterList"},
		&unstructured.UnstructuredList{},
	)
	dynFake := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
		{Group: "postgresql.cnpg.io", Version: "v1", Resource: "clusters"}: "ClusterList",
	})
	clientsetFake := fakeClientset.NewSimpleClientset()
	disc := discovery.NewFromClients(dynFake, clientsetFake, "")

	rec := New(cfg, disc)

	if rec.cfg != cfg {
		t.Errorf("Config not set correctly")
	}
	if rec.discoverer != disc {
		t.Errorf("Discoverer not set correctly")
	}
}

func TestReconcileOnce_Success(t *testing.T) {
	t.Run("writes files on successful discovery", func(t *testing.T) {
		tmpDir := t.TempDir()

		cfg := &config.Config{
			PollInterval:    30 * time.Second,
			ServersJSONPath: tmpDir + "/servers.json",
			PgpassPath:      tmpDir + "/.pgpass",
			ServerGroupName: "CNPG",
			Namespace:       "",
		}

		// Create fake K8s clients with one cluster
		scheme := runtime.NewScheme()
		scheme.AddKnownTypeWithName(
			schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "ClusterList"},
			&unstructured.UnstructuredList{},
		)

		clusters := []*unstructured.Unstructured{
			createFakeCNPGCluster("test-cluster", "default"),
		}
		secrets := []*v1.Secret{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster-superuser",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"host":     []byte("localhost"),
					"port":     []byte("5432"),
					"username": []byte("postgres"),
					"password": []byte("password"),
					"dbname":   []byte("postgres"),
					"pgpass":   []byte("localhost:5432:postgres:postgres:password"),
				},
			},
		}

		dynFake := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
			{Group: "postgresql.cnpg.io", Version: "v1", Resource: "clusters"}: "ClusterList",
		}, toRuntimeObjectSlice(clusters)...)
		clientsetFake := fakeClientset.NewSimpleClientset(toRuntimeObjectSecretsSlice(secrets)...)

		disc := discovery.NewFromClients(dynFake, clientsetFake, "")
		rec := New(cfg, disc)

		err := rec.ReconcileOnce(context.Background())
		if err != nil {
			t.Fatalf("ReconcileOnce() error = %v", err)
		}

		// Check servers.json exists
		if _, err := os.Stat(cfg.ServersJSONPath); err != nil {
			t.Errorf("servers.json not created: %v", err)
		}

		// Check .pgpass exists and has correct permissions
		info, err := os.Stat(cfg.PgpassPath)
		if err != nil {
			t.Errorf(".pgpass not created: %v", err)
		} else if info.Mode()&0777 != 0600 {
			t.Errorf(".pgpass permissions = %o, want 0600", info.Mode()&0777)
		}
	})
}

func TestReconcileOnce_EmptyClusters(t *testing.T) {
	t.Run("handles empty cluster list gracefully", func(t *testing.T) {
		tmpDir := t.TempDir()

		cfg := &config.Config{
			PollInterval:    30 * time.Second,
			ServersJSONPath: tmpDir + "/servers.json",
			PgpassPath:      tmpDir + "/.pgpass",
			ServerGroupName: "CNPG",
			Namespace:       "",
		}

		// Create fake K8s clients with no clusters
		scheme := runtime.NewScheme()
		scheme.AddKnownTypeWithName(
			schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "ClusterList"},
			&unstructured.UnstructuredList{},
		)

		dynFake := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
			{Group: "postgresql.cnpg.io", Version: "v1", Resource: "clusters"}: "ClusterList",
		})
		clientsetFake := fakeClientset.NewSimpleClientset()

		disc := discovery.NewFromClients(dynFake, clientsetFake, "")
		rec := New(cfg, disc)

		err := rec.ReconcileOnce(context.Background())
		if err != nil {
			t.Fatalf("ReconcileOnce() error = %v", err)
		}

		// Check servers.json exists and is valid JSON with empty servers
		content, err := os.ReadFile(cfg.ServersJSONPath)
		if err != nil {
			t.Fatalf("servers.json not created: %v", err)
		}
		var s map[string]interface{}
		if err := json.Unmarshal(content, &s); err != nil {
			t.Errorf("servers.json is not valid JSON: %v", err)
		}
	})
}

func TestRun_CancellationHandling(t *testing.T) {
	t.Run("stops on context cancellation", func(t *testing.T) {
		tmpDir := t.TempDir()

		cfg := &config.Config{
			PollInterval:    1 * time.Millisecond, // Very short to ensure at least one reconcile
			ServersJSONPath: tmpDir + "/servers.json",
			PgpassPath:      tmpDir + "/.pgpass",
			ServerGroupName: "CNPG",
			Namespace:       "",
		}

		// Create fake K8s clients with one cluster
		scheme := runtime.NewScheme()
		scheme.AddKnownTypeWithName(
			schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "ClusterList"},
			&unstructured.UnstructuredList{},
		)

		clusters := []*unstructured.Unstructured{
			createFakeCNPGCluster("test", "default"),
		}
		secrets := []*v1.Secret{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-superuser",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"host":     []byte("localhost"),
					"port":     []byte("5432"),
					"username": []byte("user"),
					"password": []byte("pass"),
					"dbname":   []byte("db"),
					"pgpass":   []byte("localhost:5432:db:user:pass"),
				},
			},
		}

		dynFake := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
			{Group: "postgresql.cnpg.io", Version: "v1", Resource: "clusters"}: "ClusterList",
		}, toRuntimeObjectSlice(clusters)...)
		clientsetFake := fakeClientset.NewSimpleClientset(toRuntimeObjectSecretsSlice(secrets)...)

		disc := discovery.NewFromClients(dynFake, clientsetFake, "")
		rec := New(cfg, disc)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error)
		go func() {
			done <- rec.Run(ctx)
		}()

		// Let it run briefly
		time.Sleep(50 * time.Millisecond)

		// Cancel context
		cancel()

		// Should return quickly
		select {
		case err := <-done:
			// Expected context cancellation
			if err != context.Canceled {
				t.Logf("Run() returned with: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("Run() did not exit after context cancellation")
		}
	})
}

func TestRunImmediate(t *testing.T) {
	t.Run("runs reconciliation immediately on startup", func(t *testing.T) {
		tmpDir := t.TempDir()

		cfg := &config.Config{
			PollInterval:    100 * time.Millisecond, // Long interval
			ServersJSONPath: tmpDir + "/servers.json",
			PgpassPath:      tmpDir + "/.pgpass",
			ServerGroupName: "CNPG",
			Namespace:       "",
		}

		// Create fake K8s clients with one cluster
		scheme := runtime.NewScheme()
		scheme.AddKnownTypeWithName(
			schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "ClusterList"},
			&unstructured.UnstructuredList{},
		)

		clusters := []*unstructured.Unstructured{
			createFakeCNPGCluster("test", "default"),
		}
		secrets := []*v1.Secret{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-superuser",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"host":     []byte("localhost"),
					"port":     []byte("5432"),
					"username": []byte("user"),
					"password": []byte("pass"),
					"dbname":   []byte("db"),
					"pgpass":   []byte("localhost:5432:db:user:pass"),
				},
			},
		}

		dynFake := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
			{Group: "postgresql.cnpg.io", Version: "v1", Resource: "clusters"}: "ClusterList",
		}, toRuntimeObjectSlice(clusters)...)
		clientsetFake := fakeClientset.NewSimpleClientset(toRuntimeObjectSecretsSlice(secrets)...)

		disc := discovery.NewFromClients(dynFake, clientsetFake, "")
		rec := New(cfg, disc)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go rec.Run(ctx)

		// Give it time to run the immediate reconcile
		time.Sleep(50 * time.Millisecond)

		// Check that files were written
		if _, err := os.Stat(cfg.ServersJSONPath); err != nil {
			t.Errorf("servers.json not written on startup: %v", err)
		}
		if _, err := os.Stat(cfg.PgpassPath); err != nil {
			t.Errorf(".pgpass not written on startup: %v", err)
		}
	})
}

func TestWriteFiles_EmptyClusters(t *testing.T) {
	t.Run("writes empty servers.json and .pgpass for no clusters", func(t *testing.T) {
		tmpDir := t.TempDir()

		cfg := &config.Config{
			ServersJSONPath: tmpDir + "/servers.json",
			PgpassPath:      tmpDir + "/.pgpass",
			ServerGroupName: "CNPG",
		}

		// Create fake K8s clients with no clusters
		scheme := runtime.NewScheme()
		scheme.AddKnownTypeWithName(
			schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "ClusterList"},
			&unstructured.UnstructuredList{},
		)

		dynFake := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
			{Group: "postgresql.cnpg.io", Version: "v1", Resource: "clusters"}: "ClusterList",
		})
		clientsetFake := fakeClientset.NewSimpleClientset()

		disc := discovery.NewFromClients(dynFake, clientsetFake, "")
		rec := New(cfg, disc)

		rec.writeFiles([]discovery.ClusterInfo{})

		// Check servers.json exists and is valid JSON
		serverContent, err := os.ReadFile(cfg.ServersJSONPath)
		if err != nil {
			t.Fatalf("servers.json not created: %v", err)
		}
		if !isValidJSON(serverContent) {
			t.Errorf("servers.json is not valid JSON: %s", string(serverContent))
		}

		// Check .pgpass exists (may be empty)
		pgpassContent, err := os.ReadFile(cfg.PgpassPath)
		if err != nil {
			t.Fatalf(".pgpass not created: %v", err)
		}
		if len(pgpassContent) != 0 {
			t.Errorf(".pgpass should be empty for no clusters, got %q", string(pgpassContent))
		}
	})
}

func TestReconcile_UnchangedConfigurationSkipsRewrite(t *testing.T) {
	t.Run("does not rewrite files when discovered configuration is unchanged", func(t *testing.T) {
		tmpDir := t.TempDir()

		cfg := &config.Config{
			PollInterval:    30 * time.Second,
			ServersJSONPath: tmpDir + "/servers.json",
			PgpassPath:      tmpDir + "/.pgpass",
			ServerGroupName: "CNPG",
			Namespace:       "",
		}

		scheme := runtime.NewScheme()
		scheme.AddKnownTypeWithName(
			schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "ClusterList"},
			&unstructured.UnstructuredList{},
		)

		clusters := []*unstructured.Unstructured{
			createFakeCNPGCluster("stable-cluster", "default"),
		}
		secrets := []*v1.Secret{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "stable-cluster-superuser",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"host":     []byte("localhost"),
					"port":     []byte("5432"),
					"username": []byte("postgres"),
					"password": []byte("password"),
					"dbname":   []byte("postgres"),
					"pgpass":   []byte("localhost:5432:postgres:postgres:password"),
				},
			},
		}

		dynFake := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
			{Group: "postgresql.cnpg.io", Version: "v1", Resource: "clusters"}: "ClusterList",
		}, toRuntimeObjectSlice(clusters)...)
		clientsetFake := fakeClientset.NewSimpleClientset(toRuntimeObjectSecretsSlice(secrets)...)

		disc := discovery.NewFromClients(dynFake, clientsetFake, "")
		rec := New(cfg, disc)

		rec.reconcile(context.Background())

		serversInfoBefore, err := os.Stat(cfg.ServersJSONPath)
		if err != nil {
			t.Fatalf("servers.json should exist after first reconcile: %v", err)
		}
		pgpassInfoBefore, err := os.Stat(cfg.PgpassPath)
		if err != nil {
			t.Fatalf(".pgpass should exist after first reconcile: %v", err)
		}

		time.Sleep(20 * time.Millisecond)

		rec.reconcile(context.Background())

		serversInfoAfter, err := os.Stat(cfg.ServersJSONPath)
		if err != nil {
			t.Fatalf("servers.json should exist after second reconcile: %v", err)
		}
		pgpassInfoAfter, err := os.Stat(cfg.PgpassPath)
		if err != nil {
			t.Fatalf(".pgpass should exist after second reconcile: %v", err)
		}

		if !serversInfoAfter.ModTime().Equal(serversInfoBefore.ModTime()) {
			t.Errorf("servers.json was rewritten for unchanged configuration")
		}
		if !pgpassInfoAfter.ModTime().Equal(pgpassInfoBefore.ModTime()) {
			t.Errorf(".pgpass was rewritten for unchanged configuration")
		}
	})
}

// Helper function
func isValidJSON(b []byte) bool {
	var tmp interface{}
	return nil == json.Unmarshal(b, &tmp)
}

func TestReconcile_RestartsPodOnConfigChange(t *testing.T) {
	t.Run("deletes pod when configuration changes after initial load", func(t *testing.T) {
		tmpDir := t.TempDir()

		cfg := &config.Config{
			PollInterval:    30 * time.Second,
			ServersJSONPath: tmpDir + "/servers.json",
			PgpassPath:      tmpDir + "/.pgpass",
			ServerGroupName: "CNPG",
			Namespace:       "",
			PodName:         "pgadmin-pod",
			PodNamespace:    "default",
		}

		scheme := runtime.NewScheme()
		scheme.AddKnownTypeWithName(
			schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "ClusterList"},
			&unstructured.UnstructuredList{},
		)

		clusters := []*unstructured.Unstructured{
			createFakeCNPGCluster("cluster-a", "default"),
		}
		secrets := []*v1.Secret{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cluster-a-superuser",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"host":     []byte("localhost"),
					"port":     []byte("5432"),
					"username": []byte("postgres"),
					"password": []byte("password"),
					"dbname":   []byte("postgres"),
					"pgpass":   []byte("localhost:5432:postgres:postgres:password"),
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cluster-b-superuser",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"host":     []byte("host-b"),
					"port":     []byte("5432"),
					"username": []byte("postgres"),
					"password": []byte("password"),
					"dbname":   []byte("postgres"),
					"pgpass":   []byte("host-b:5432:postgres:postgres:password"),
				},
			},
		}
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pgadmin-pod",
				Namespace: "default",
			},
		}

		dynFake := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
			{Group: "postgresql.cnpg.io", Version: "v1", Resource: "clusters"}: "ClusterList",
		}, toRuntimeObjectSlice(clusters)...)
		clientsetFake := fakeClientset.NewSimpleClientset(append(toRuntimeObjectSecretsSlice(secrets), pod)...)

		disc := discovery.NewFromClients(dynFake, clientsetFake, "")
		rec := New(cfg, disc)

		// First reconcile: initial load — pod must NOT be deleted
		rec.reconcile(context.Background())

		_, err := clientsetFake.CoreV1().Pods("default").Get(context.Background(), "pgadmin-pod", metav1.GetOptions{})
		if err != nil {
			t.Fatalf("pod should not be deleted on initial load: %v", err)
		}

		// Add a second cluster to trigger a config change
		clusterB := createFakeCNPGCluster("cluster-b", "default")
		if _, err := dynFake.Resource(schema.GroupVersionResource{
			Group: "postgresql.cnpg.io", Version: "v1", Resource: "clusters",
		}).Namespace("default").Create(context.Background(), clusterB, metav1.CreateOptions{}); err != nil {
			t.Fatalf("failed to add cluster-b: %v", err)
		}

		// Second reconcile: config changed — pod must be deleted
		rec.reconcile(context.Background())

		_, err = clientsetFake.CoreV1().Pods("default").Get(context.Background(), "pgadmin-pod", metav1.GetOptions{})
		if err == nil {
			t.Errorf("pod should have been deleted after config change")
		}
	})

	t.Run("does not delete pod when POD_NAME is not configured", func(t *testing.T) {
		tmpDir := t.TempDir()

		cfg := &config.Config{
			PollInterval:    30 * time.Second,
			ServersJSONPath: tmpDir + "/servers.json",
			PgpassPath:      tmpDir + "/.pgpass",
			ServerGroupName: "CNPG",
			Namespace:       "",
			PodName:         "", // not configured
			PodNamespace:    "default",
		}

		scheme := runtime.NewScheme()
		scheme.AddKnownTypeWithName(
			schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "ClusterList"},
			&unstructured.UnstructuredList{},
		)

		clusters := []*unstructured.Unstructured{
			createFakeCNPGCluster("cluster-a", "default"),
		}
		secrets := []*v1.Secret{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cluster-a-superuser",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"host":     []byte("localhost"),
					"port":     []byte("5432"),
					"username": []byte("postgres"),
					"password": []byte("password"),
					"dbname":   []byte("postgres"),
					"pgpass":   []byte("localhost:5432:postgres:postgres:password"),
				},
			},
		}
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pgadmin-pod",
				Namespace: "default",
			},
		}

		dynFake := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
			{Group: "postgresql.cnpg.io", Version: "v1", Resource: "clusters"}: "ClusterList",
		}, toRuntimeObjectSlice(clusters)...)
		clientsetFake := fakeClientset.NewSimpleClientset(append(toRuntimeObjectSecretsSlice(secrets), pod)...)

		disc := discovery.NewFromClients(dynFake, clientsetFake, "")
		rec := New(cfg, disc)

		// Simulate that initial config was already applied
		rec.hasAppliedConfig = true
		rec.lastAppliedConfigHash = "old-hash"

		rec.reconcile(context.Background())

		// Pod must NOT be deleted because PodName is empty
		_, err := clientsetFake.CoreV1().Pods("default").Get(context.Background(), "pgadmin-pod", metav1.GetOptions{})
		if err != nil {
			t.Errorf("pod should not be deleted when POD_NAME is not configured: %v", err)
		}
	})
}

func newFakeDynClientWithListError(t *testing.T) *fake.FakeDynamicClient {
	t.Helper()
	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "ClusterList"},
		&unstructured.UnstructuredList{},
	)
	dynFake := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
		{Group: "postgresql.cnpg.io", Version: "v1", Resource: "clusters"}: "ClusterList",
	})
	dynFake.Fake.PrependReactor("list", "clusters", func(action ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("simulated list failure")
	})
	return dynFake
}

func TestReconcileOnce_DiscoverError(t *testing.T) {
	t.Run("returns error when discovery fails", func(t *testing.T) {
		tmpDir := t.TempDir()

		cfg := &config.Config{
			ServersJSONPath: tmpDir + "/servers.json",
			PgpassPath:      tmpDir + "/.pgpass",
			ServerGroupName: "CNPG",
		}

		disc := discovery.NewFromClients(newFakeDynClientWithListError(t), fakeClientset.NewSimpleClientset(), "")
		rec := New(cfg, disc)

		err := rec.ReconcileOnce(context.Background())
		if err == nil {
			t.Errorf("ReconcileOnce() expected error, got nil")
		}
	})
}

func TestWriteFiles_Error(t *testing.T) {
	t.Run("returns false when servers.json write fails", func(t *testing.T) {
		cfg := &config.Config{
			ServersJSONPath: "/dev/null/servers.json",
			PgpassPath:      "/dev/null/.pgpass",
			ServerGroupName: "CNPG",
		}

		scheme := runtime.NewScheme()
		dynFake := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, nil)
		disc := discovery.NewFromClients(dynFake, fakeClientset.NewSimpleClientset(), "")
		rec := New(cfg, disc)

		if result := rec.writeFiles([]discovery.ClusterInfo{}); result {
			t.Errorf("writeFiles() = true, want false when WriteServersJSON fails")
		}
	})

	t.Run("returns false when pgpass write fails", func(t *testing.T) {
		tmpDir := t.TempDir()

		cfg := &config.Config{
			ServersJSONPath: tmpDir + "/servers.json",
			PgpassPath:      "/dev/null/.pgpass",
			ServerGroupName: "CNPG",
		}

		scheme := runtime.NewScheme()
		dynFake := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, nil)
		disc := discovery.NewFromClients(dynFake, fakeClientset.NewSimpleClientset(), "")
		rec := New(cfg, disc)

		if result := rec.writeFiles([]discovery.ClusterInfo{}); result {
			t.Errorf("writeFiles() = true, want false when WritePgpass fails")
		}
	})
}

func TestReconcile_DiscoverError(t *testing.T) {
	t.Run("logs error and does not write files when discovery fails", func(t *testing.T) {
		tmpDir := t.TempDir()

		cfg := &config.Config{
			PollInterval:    30 * time.Second,
			ServersJSONPath: tmpDir + "/servers.json",
			PgpassPath:      tmpDir + "/.pgpass",
			ServerGroupName: "CNPG",
		}

		disc := discovery.NewFromClients(newFakeDynClientWithListError(t), fakeClientset.NewSimpleClientset(), "")
		rec := New(cfg, disc)

		rec.reconcile(context.Background())

		// Files must not be created because discovery failed before write.
		if _, err := os.Stat(cfg.ServersJSONPath); err == nil {
			t.Errorf("servers.json should not be created when discovery fails")
		}
		if _, err := os.Stat(cfg.PgpassPath); err == nil {
			t.Errorf(".pgpass should not be created when discovery fails")
		}
	})
}

func TestRestartPod_DeleteError(t *testing.T) {
	t.Run("logs error when pod deletion fails", func(t *testing.T) {
		cfg := &config.Config{
			PodName:      "nonexistent-pod",
			PodNamespace: "default",
		}

		scheme := runtime.NewScheme()
		dynFake := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, nil)
		// Clientset has no pods, so Delete will return a not-found error.
		disc := discovery.NewFromClients(dynFake, fakeClientset.NewSimpleClientset(), "")
		rec := New(cfg, disc)

		// Must not panic even when the pod does not exist.
		rec.restartPod(context.Background())
	})
}
