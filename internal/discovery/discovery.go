package discovery

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var cnpgClusterGVR = schema.GroupVersionResource{
	Group:    "postgresql.cnpg.io",
	Version:  "v1",
	Resource: "clusters",
}

// Discoverer finds CNPG clusters and reads their connection secrets.
type Discoverer struct {
	dynamicClient dynamic.Interface
	clientset     kubernetes.Interface
	namespace     string // empty = all namespaces
}

// New creates a Discoverer using in-cluster Kubernetes config.
func New(namespace string) (*Discoverer, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("building in-cluster config: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating dynamic client: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating kubernetes clientset: %w", err)
	}

	return &Discoverer{
		dynamicClient: dynClient,
		clientset:     clientset,
		namespace:     namespace,
	}, nil
}

// NewFromClients creates a Discoverer from pre-built clients (useful for testing).
func NewFromClients(dynClient dynamic.Interface, clientset kubernetes.Interface, namespace string) *Discoverer {
	return &Discoverer{
		dynamicClient: dynClient,
		clientset:     clientset,
		namespace:     namespace,
	}
}

// Clientset returns the underlying Kubernetes clientset.
func (d *Discoverer) Clientset() kubernetes.Interface {
	return d.clientset
}

// DiscoverClusters lists all CNPG clusters and reads their superuser secrets.
func (d *Discoverer) DiscoverClusters(ctx context.Context) ([]ClusterInfo, error) {
	list, err := d.dynamicClient.Resource(cnpgClusterGVR).Namespace(d.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing CNPG clusters: %w", err)
	}

	var clusters []ClusterInfo
	for _, item := range list.Items {
		name := item.GetName()
		ns := item.GetNamespace()

		info, err := d.readClusterSecret(ctx, name, ns)
		if err != nil {
			slog.Warn("skipping cluster: could not read secret", "cluster", name, "namespace", ns, "error", err)
			continue
		}
		clusters = append(clusters, *info)
	}

	return clusters, nil
}

// readClusterSecret tries to read the superuser secret first, then falls back to the app secret.
func (d *Discoverer) readClusterSecret(ctx context.Context, clusterName, namespace string) (*ClusterInfo, error) {
	secretNames := []string{
		clusterName + "-superuser",
		clusterName + "-app",
	}

	for _, secretName := range secretNames {
		secret, err := d.clientset.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
		if err != nil {
			slog.Debug("secret not found, trying next", "secret", secretName, "namespace", namespace, "error", err)
			continue
		}

		info := &ClusterInfo{
			Name:      clusterName,
			Namespace: namespace,
			Host:      string(secret.Data["host"]),
			Port:      string(secret.Data["port"]),
			Username:  string(secret.Data["username"]),
			Password:  string(secret.Data["password"]),
			Database:  string(secret.Data["dbname"]),
			Pgpass:    strings.TrimSpace(string(secret.Data["pgpass"])),
		}

		if info.Host == "" || info.Username == "" || info.Pgpass == "" {
			slog.Warn("secret missing required fields", "secret", secretName, "namespace", namespace)
			continue
		}

		// Default port if not set
		if info.Port == "" {
			info.Port = "5432"
		}
		// Default database if not set
		if info.Database == "" {
			info.Database = "postgres"
		}

		slog.Debug("discovered cluster", "cluster", clusterName, "namespace", namespace, "secret", secretName)
		return info, nil
	}

	return nil, fmt.Errorf("no valid secret found for cluster %s/%s", namespace, clusterName)
}
