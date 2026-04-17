package discovery

import "testing"

func TestClusterInfo_ServerKey(t *testing.T) {
	tests := []struct {
		name    string
		cluster ClusterInfo
		want    string
	}{
		{
			name: "basic cluster key",
			cluster: ClusterInfo{
				Name:      "my-cluster",
				Namespace: "default",
			},
			want: "default/my-cluster",
		},
		{
			name: "cluster with special chars",
			cluster: ClusterInfo{
				Name:      "prod-cluster-01",
				Namespace: "production",
			},
			want: "production/prod-cluster-01",
		},
		{
			name: "single namespace",
			cluster: ClusterInfo{
				Name:      "test",
				Namespace: "ns",
			},
			want: "ns/test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cluster.ServerKey()
			if got != tt.want {
				t.Errorf("ServerKey() = %q, want %q", got, tt.want)
			}
		})
	}
}
