package discovery

import (
	"context"
	"fmt"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	fakeClientset "k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

func TestDiscoverer_DiscoverClusters(t *testing.T) {
	tests := []struct {
		name            string
		clusters        []*unstructured.Unstructured
		secrets         []*v1.Secret
		namespace       string
		wantCount       int
		wantClusterName string
		wantErr         bool
	}{
		{
			name:      "no clusters",
			clusters:  []*unstructured.Unstructured{},
			secrets:   []*v1.Secret{},
			wantCount: 0,
			wantErr:   false,
		},
		{
			name: "single cluster with superuser secret",
			clusters: []*unstructured.Unstructured{
				createFakeCNPGCluster("test-cluster", "default"),
			},
			secrets: []*v1.Secret{
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-cluster-superuser",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"host":     []byte("postgres.example.com"),
						"port":     []byte("5432"),
						"username": []byte("postgres"),
						"password": []byte("secretpass"),
						"dbname":   []byte("postgres"),
						"pgpass":   []byte("postgres.example.com:5432:postgres:postgres:secretpass"),
					},
				},
			},
			namespace:       "",
			wantCount:       1,
			wantClusterName: "test-cluster",
			wantErr:         false,
		},
		{
			name: "cluster with app secret fallback",
			clusters: []*unstructured.Unstructured{
				createFakeCNPGCluster("my-cluster", "prod"),
			},
			secrets: []*v1.Secret{
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-cluster-app",
						Namespace: "prod",
					},
					Data: map[string][]byte{
						"host":     []byte("db.internal"),
						"port":     []byte("5432"),
						"username": []byte("app_user"),
						"password": []byte("apppass"),
						"dbname":   []byte("appdb"),
						"pgpass":   []byte("db.internal:5432:appdb:app_user:apppass"),
					},
				},
			},
			namespace:       "",
			wantCount:       1,
			wantClusterName: "my-cluster",
			wantErr:         false,
		},
		{
			name: "multiple clusters in different namespaces",
			clusters: []*unstructured.Unstructured{
				createFakeCNPGCluster("cluster-1", "ns1"),
				createFakeCNPGCluster("cluster-2", "ns2"),
			},
			secrets: []*v1.Secret{
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cluster-1-superuser",
						Namespace: "ns1",
					},
					Data: map[string][]byte{
						"host":     []byte("db1.local"),
						"port":     []byte("5432"),
						"username": []byte("user1"),
						"password": []byte("pass1"),
						"dbname":   []byte("db1"),
						"pgpass":   []byte("db1.local:5432:db1:user1:pass1"),
					},
				},
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cluster-2-superuser",
						Namespace: "ns2",
					},
					Data: map[string][]byte{
						"host":     []byte("db2.local"),
						"port":     []byte("5432"),
						"username": []byte("user2"),
						"password": []byte("pass2"),
						"dbname":   []byte("db2"),
						"pgpass":   []byte("db2.local:5432:db2:user2:pass2"),
					},
				},
			},
			namespace: "",
			wantCount: 2,
			wantErr:   false,
		},
		{
			name: "namespace filter",
			clusters: []*unstructured.Unstructured{
				createFakeCNPGCluster("cluster-1", "prod"),
				createFakeCNPGCluster("cluster-2", "dev"),
			},
			secrets: []*v1.Secret{
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cluster-1-superuser",
						Namespace: "prod",
					},
					Data: map[string][]byte{
						"host":     []byte("db1.local"),
						"port":     []byte("5432"),
						"username": []byte("user1"),
						"password": []byte("pass1"),
						"dbname":   []byte("db1"),
						"pgpass":   []byte("db1.local:5432:db1:user1:pass1"),
					},
				},
			},
			namespace: "prod",
			wantCount: 1,
			wantErr:   false,
		},
		{
			name: "cluster without secret is skipped",
			clusters: []*unstructured.Unstructured{
				createFakeCNPGCluster("no-secret-cluster", "default"),
			},
			secrets:   []*v1.Secret{},
			wantCount: 0,
			wantErr:   false,
		},
		{
			name: "secret missing required fields is skipped",
			clusters: []*unstructured.Unstructured{
				createFakeCNPGCluster("incomplete", "default"),
			},
			secrets: []*v1.Secret{
				&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "incomplete-superuser",
						Namespace: "default",
					},
					Data: map[string][]byte{
						// Missing one or more required fields ('host', 'username', 'pgpass')
						"dbname": []byte("db"),
					},
				},
			},
			wantCount: 0,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			// Register the CNPG cluster as a list kind for the fake client
			scheme.AddKnownTypeWithName(
				schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "ClusterList"},
				&unstructured.UnstructuredList{},
			)
			dynFake := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
				cnpgClusterGVR: "ClusterList",
			}, toRuntimeObject(tt.clusters)...)
			clientsetFake := fakeClientset.NewSimpleClientset(toRuntimeObject(tt.secrets)...)

			disc := NewFromClients(dynFake, clientsetFake, tt.namespace)
			got, err := disc.DiscoverClusters(context.Background())

			if (err != nil) != tt.wantErr {
				t.Errorf("DiscoverClusters() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if len(got) != tt.wantCount {
				t.Errorf("DiscoverClusters() len = %d, want %d", len(got), tt.wantCount)
			}

			if tt.wantClusterName != "" && len(got) > 0 {
				if got[0].Name != tt.wantClusterName {
					t.Errorf("DiscoverClusters()[0].Name = %q, want %q", got[0].Name, tt.wantClusterName)
				}
			}
		})
	}
}

func TestDiscoverer_DefaultPorts(t *testing.T) {
	t.Run("port defaults to 5432 when empty", func(t *testing.T) {
		scheme := runtime.NewScheme()
		scheme.AddKnownTypeWithName(
			schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "ClusterList"},
			&unstructured.UnstructuredList{},
		)
		clusters := []*unstructured.Unstructured{
			createFakeCNPGCluster("test", "default"),
		}
		secrets := []*v1.Secret{
			&v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-superuser",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"host":     []byte("localhost"),
					"port":     []byte(""), // Empty port
					"username": []byte("user"),
					"password": []byte("pass"),
					"dbname":   []byte("db"),
					"pgpass":   []byte("localhost:5432:db:user:pass"),
				},
			},
		}

		dynFake := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
			cnpgClusterGVR: "ClusterList",
		}, toRuntimeObject(clusters)...)
		clientsetFake := fakeClientset.NewSimpleClientset(toRuntimeObject(secrets)...)

		disc := NewFromClients(dynFake, clientsetFake, "")
		got, err := disc.DiscoverClusters(context.Background())

		if err != nil {
			t.Fatalf("DiscoverClusters() error = %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("DiscoverClusters() len = %d, want 1", len(got))
		}
		if got[0].Port != "5432" {
			t.Errorf("Port = %q, want %q", got[0].Port, "5432")
		}
	})

	t.Run("database defaults to postgres when empty", func(t *testing.T) {
		scheme := runtime.NewScheme()
		scheme.AddKnownTypeWithName(
			schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "ClusterList"},
			&unstructured.UnstructuredList{},
		)
		clusters := []*unstructured.Unstructured{
			createFakeCNPGCluster("test", "default"),
		}
		secrets := []*v1.Secret{
			&v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-superuser",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"host":     []byte("localhost"),
					"port":     []byte("5432"),
					"username": []byte("user"),
					"password": []byte("pass"),
					"dbname":   []byte(""), // Empty dbname
					"pgpass":   []byte("localhost:5432:postgres:user:pass"),
				},
			},
		}

		dynFake := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
			cnpgClusterGVR: "ClusterList",
		}, toRuntimeObject(clusters)...)
		clientsetFake := fakeClientset.NewSimpleClientset(toRuntimeObject(secrets)...)

		disc := NewFromClients(dynFake, clientsetFake, "")
		got, err := disc.DiscoverClusters(context.Background())

		if err != nil {
			t.Fatalf("DiscoverClusters() error = %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("DiscoverClusters() len = %d, want 1", len(got))
		}
		if got[0].Database != "postgres" {
			t.Errorf("Database = %q, want %q", got[0].Database, "postgres")
		}
	})
}

// Helper functions

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

func TestDiscoverer_Clientset(t *testing.T) {
	scheme := runtime.NewScheme()
	dynFake := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, nil)
	clientsetFake := fakeClientset.NewSimpleClientset()

	d := NewFromClients(dynFake, clientsetFake, "")

	if got := d.Clientset(); got != clientsetFake {
		t.Errorf("Clientset() = %v, want %v", got, clientsetFake)
	}
}

func TestDiscoverer_DiscoverClusters_ListError(t *testing.T) {
	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "ClusterList"},
		&unstructured.UnstructuredList{},
	)
	dynFake := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
		cnpgClusterGVR: "ClusterList",
	})
	dynFake.Fake.PrependReactor("list", "clusters", func(action ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("simulated list failure")
	})
	clientsetFake := fakeClientset.NewSimpleClientset()

	disc := NewFromClients(dynFake, clientsetFake, "")
	_, err := disc.DiscoverClusters(context.Background())

	if err == nil {
		t.Errorf("DiscoverClusters() expected error, got nil")
	}
}

func toRuntimeObject(objs interface{}) []runtime.Object {
	switch v := objs.(type) {
	case []*unstructured.Unstructured:
		result := make([]runtime.Object, len(v))
		for i, obj := range v {
			result[i] = obj
		}
		return result
	case []*v1.Secret:
		result := make([]runtime.Object, len(v))
		for i, obj := range v {
			result[i] = obj
		}
		return result
	default:
		return nil
	}
}
