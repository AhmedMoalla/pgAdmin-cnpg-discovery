package pgadmin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewAPIClient(t *testing.T) {
	t.Run("creates client with provided credentials", func(t *testing.T) {
		client := NewAPIClient("http://pgadmin.local", "admin@example.com", "password")

		if client.baseURL != "http://pgadmin.local" {
			t.Errorf("baseURL = %q, want %q", client.baseURL, "http://pgadmin.local")
		}
		if client.email != "admin@example.com" {
			t.Errorf("email = %q, want %q", client.email, "admin@example.com")
		}
		if client.password != "password" {
			t.Errorf("password = %q, want %q", client.password, "password")
		}
	})

	t.Run("removes trailing slash from baseURL", func(t *testing.T) {
		client := NewAPIClient("http://pgadmin.local/", "admin@example.com", "password")

		if client.baseURL != "http://pgadmin.local" {
			t.Errorf("baseURL = %q, want %q", client.baseURL, "http://pgadmin.local")
		}
	})
}

func TestExtractCSRFToken(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "csrf_token input field",
			body: `<input id="csrf_token" name="csrf_token" type="hidden" value="abc123xyz">`,
			want: "abc123xyz",
		},
		{
			name: "csrf_token input with attributes reordered",
			body: `<input value="reordered_token" type="hidden" name="csrf_token" id="csrf_token">`,
			want: "reordered_token",
		},
		{
			name: "csrfToken meta tag",
			body: `<meta name="csrfToken" content="meta_token_value">`,
			want: "meta_token_value",
		},
		{
			name: "no token found",
			body: `<html><body>test</body></html>`,
			want: "",
		},
		{
			name: "empty value",
			body: `<input name="csrf_token" type="hidden" value="">`,
			want: "",
		},
		{
			name: "token with special characters",
			body: `<input id="csrf_token" name="csrf_token" type="hidden" value="a1b2c3-d4e5_f6g7">`,
			want: "a1b2c3-d4e5_f6g7",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractCSRFToken(tt.body)
			if got != tt.want {
				t.Errorf("extractCSRFToken() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLogin(t *testing.T) {
	t.Run("successful login", func(t *testing.T) {
		csrfPosted := false
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "/login") && r.Method == "GET" {
				w.Header().Set("Content-Type", "text/html")
				fmt.Fprint(w, `<input name="csrf_token" type="hidden" value="test_token">`)
				return
			}
			if strings.Contains(r.URL.Path, "/authenticate/login") && r.Method == "POST" {
				if err := r.ParseForm(); err == nil && r.FormValue("csrf_token") == "test_token" {
					csrfPosted = true
				}
				http.SetCookie(w, &http.Cookie{Name: "session", Value: "abc123"})
				http.Redirect(w, r, "/browser/", http.StatusFound)
				return
			}
			if strings.Contains(r.URL.Path, "/browser/") {
				w.Header().Set("Content-Type", "text/html")
				fmt.Fprint(w, `<input name="csrf_token" type="hidden" value="refreshed_token">`)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewAPIClient(server.URL, "test@test.com", "password")
		err := client.Login()

		if err != nil {
			t.Fatalf("Login() error = %v", err)
		}
		if !client.loggedIn {
			t.Errorf("loggedIn = false, want true")
		}
		if !csrfPosted {
			t.Errorf("login POST missing expected csrf_token form field")
		}
	})

	t.Run("successful login with csrf token from cookie", func(t *testing.T) {
		csrfPosted := false
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "/login") && r.Method == "GET" {
				http.SetCookie(w, &http.Cookie{Name: "csrf_token", Value: "cookie_token"})
				w.Header().Set("Content-Type", "text/html")
				fmt.Fprint(w, `<html><body>login</body></html>`)
				return
			}
			if strings.Contains(r.URL.Path, "/authenticate/login") && r.Method == "POST" {
				if err := r.ParseForm(); err == nil && r.FormValue("csrf_token") == "cookie_token" {
					csrfPosted = true
				}
				http.Redirect(w, r, "/browser/", http.StatusFound)
				return
			}
			if strings.Contains(r.URL.Path, "/browser/") {
				w.Header().Set("Content-Type", "text/html")
				fmt.Fprint(w, `<input name="csrf_token" type="hidden" value="refreshed_token">`)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewAPIClient(server.URL, "test@test.com", "password")
		err := client.Login()

		if err != nil {
			t.Fatalf("Login() error = %v", err)
		}
		if !client.loggedIn {
			t.Errorf("loggedIn = false, want true")
		}
		if !csrfPosted {
			t.Errorf("login POST missing csrf token from cookie fallback")
		}
	})

	t.Run("login failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer server.Close()

		client := NewAPIClient(server.URL, "test@test.com", "password")
		err := client.Login()

		if err == nil {
			t.Errorf("Login() error = nil, want error")
		}
		if client.loggedIn {
			t.Errorf("loggedIn = true, want false")
		}
	})

	t.Run("login rejected when still on login page", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "/login") && r.Method == "GET" {
				w.Header().Set("Content-Type", "text/html")
				fmt.Fprint(w, `<input name="csrf_token" type="hidden" value="test_token">`)
				return
			}
			if strings.Contains(r.URL.Path, "/authenticate/login") && r.Method == "POST" {
				// Simulate pgAdmin returning login page again (auth failed).
				w.Header().Set("Content-Type", "text/html")
				fmt.Fprint(w, `<input name="csrf_token" type="hidden" value="new_token">`)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewAPIClient(server.URL, "test@test.com", "password")
		err := client.Login()

		if err == nil {
			t.Fatalf("Login() error = nil, want error")
		}
		if client.loggedIn {
			t.Errorf("loggedIn = true, want false")
		}
	})
}

func TestIsAvailable(t *testing.T) {
	t.Run("pgadmin available", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "/misc/ping") {
				w.WriteHeader(http.StatusOK)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewAPIClient(server.URL, "admin", "pass")
		got := client.IsAvailable()

		if !got {
			t.Errorf("IsAvailable() = false, want true")
		}
	})

	t.Run("pgadmin not available", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer server.Close()

		client := NewAPIClient(server.URL, "admin", "pass")
		got := client.IsAvailable()

		if got {
			t.Errorf("IsAvailable() = true, want false")
		}
	})

	t.Run("pgadmin unreachable", func(t *testing.T) {
		client := NewAPIClient("http://unreachable.local.invalid:99999", "admin", "pass")
		got := client.IsAvailable()

		if got {
			t.Errorf("IsAvailable() = true, want false")
		}
	})
}

func TestListServerGroups(t *testing.T) {
	t.Run("lists server groups successfully", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "/browser/server_group/obj/") {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode([]struct {
					ID   int    `json:"id"`
					Name string `json:"name"`
				}{
					{ID: 1, Name: "Servers"},
					{ID: 2, Name: "CNPG Clusters"},
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewAPIClient(server.URL, "admin", "pass")
		client.loggedIn = true // Skip login

		got, err := client.ListServerGroups()
		if err != nil {
			t.Fatalf("ListServerGroups() error = %v", err)
		}

		if len(got) != 2 {
			t.Fatalf("ListServerGroups() len = %d, want 2", len(got))
		}
		if got[0].Name != "Servers" {
			t.Errorf("Group 0 Name = %q, want %q", got[0].Name, "Servers")
		}
		if got[1].ID != 2 {
			t.Errorf("Group 1 ID = %d, want %d", got[1].ID, 2)
		}
	})

	t.Run("requires login", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewAPIClient(server.URL, "admin", "pass")
		client.loggedIn = false

		_, err := client.ListServerGroups()
		if err == nil {
			t.Errorf("ListServerGroups() error = nil, want error (not logged in)")
		}
	})
}

func TestFindOrCreateServerGroup(t *testing.T) {
	t.Run("finds existing group", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "/browser/server_group/obj/") && r.Method == "GET" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode([]struct {
					ID   int    `json:"id"`
					Name string `json:"name"`
				}{
					{ID: 5, Name: "CNPG Clusters"},
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewAPIClient(server.URL, "admin", "pass")
		client.loggedIn = true

		got, err := client.FindOrCreateServerGroup("CNPG Clusters")
		if err != nil {
			t.Fatalf("FindOrCreateServerGroup() error = %v", err)
		}

		if got != 5 {
			t.Errorf("FindOrCreateServerGroup() = %d, want 5", got)
		}
	})

	t.Run("creates group if not found", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "/browser/server_group/obj/") && r.Method == "GET" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode([]struct {
					ID   int    `json:"id"`
					Name string `json:"name"`
				}{
					{ID: 1, Name: "Servers"},
				})
				return
			}
			if strings.Contains(r.URL.Path, "/browser/server_group/obj/") && r.Method == "POST" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(struct {
					Node struct {
						ID int `json:"_id"`
					} `json:"node"`
				}{
					Node: struct {
						ID int `json:"_id"`
					}{ID: 10},
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewAPIClient(server.URL, "admin", "pass")
		client.loggedIn = true

		got, err := client.FindOrCreateServerGroup("CNPG Clusters")
		if err != nil {
			t.Fatalf("FindOrCreateServerGroup() error = %v", err)
		}

		if got != 10 {
			t.Errorf("FindOrCreateServerGroup() = %d, want 10", got)
		}
	})
}

func TestCreateServer(t *testing.T) {
	t.Run("creates server successfully", func(t *testing.T) {
		createdCalled := false
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "/browser/server/obj/") && r.Method == "POST" {
				createdCalled = true
				w.WriteHeader(http.StatusCreated)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewAPIClient(server.URL, "admin", "pass")
		client.loggedIn = true

		entry := ServerEntry{
			Name:          "test-cluster",
			Host:          "localhost",
			Port:          5432,
			MaintenanceDB: "postgres",
			Username:      "user",
			SSLMode:       "prefer",
		}

		err := client.CreateServer(1, entry)
		if err != nil {
			t.Fatalf("CreateServer() error = %v", err)
		}
		if !createdCalled {
			t.Errorf("POST /browser/server/obj/ not called")
		}
	})

	t.Run("create server failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer server.Close()

		client := NewAPIClient(server.URL, "admin", "pass")
		client.loggedIn = true

		entry := ServerEntry{Name: "test"}
		err := client.CreateServer(1, entry)

		if err == nil {
			t.Errorf("CreateServer() error = nil, want error")
		}
	})
}

func TestDeleteServer(t *testing.T) {
	t.Run("deletes server successfully", func(t *testing.T) {
		deletedCalled := false
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "/browser/server/obj/") && r.Method == "DELETE" {
				deletedCalled = true
				w.WriteHeader(http.StatusOK)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewAPIClient(server.URL, "admin", "pass")
		client.loggedIn = true

		err := client.DeleteServer(1, 5)
		if err != nil {
			t.Fatalf("DeleteServer() error = %v", err)
		}
		if !deletedCalled {
			t.Errorf("DELETE /browser/server/obj/ not called")
		}
	})

	t.Run("delete server failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewAPIClient(server.URL, "admin", "pass")
		client.loggedIn = true

		err := client.DeleteServer(1, 5)
		if err == nil {
			t.Errorf("DeleteServer() error = nil, want error")
		}
	})
}
