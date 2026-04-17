package pgadmin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

// APIClient communicates with pgAdmin's internal REST API.
type APIClient struct {
	baseURL   string
	email     string
	password  string
	client    *http.Client
	csrfToken string
	loggedIn  bool
}

// APIServer represents a server as returned by the pgAdmin API.
type APIServer struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Comment string `json:"comment,omitempty"`
	Group   int    `json:"gid"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
}

// NewAPIClient creates a new pgAdmin API client.
func NewAPIClient(baseURL, email, password string) *APIClient {
	jar, _ := cookiejar.New(nil)
	return &APIClient{
		baseURL:  strings.TrimRight(baseURL, "/"),
		email:    email,
		password: password,
		client: &http.Client{
			Jar:     jar,
			Timeout: 15 * time.Second,
		},
	}
}

// Login authenticates with pgAdmin and stores the session cookie and CSRF token.
func (a *APIClient) Login() error {
	// First, GET the login page to obtain the CSRF token
	loginURL := a.baseURL + "/login"
	resp, err := a.client.Get(loginURL)
	if err != nil {
		return fmt.Errorf("getting login page: %w", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	// Extract CSRF token from the page or cookies
	a.csrfToken = extractCSRFToken(string(body))

	// POST login credentials
	form := url.Values{
		"email":    {a.email},
		"password": {a.password},
	}

	req, err := http.NewRequest("POST", loginURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("creating login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if a.csrfToken != "" {
		req.Header.Set("X-CSRFToken", a.csrfToken)
		req.Header.Set("X-pgA-CSRFToken", a.csrfToken)
	}

	resp, err = a.client.Do(req)
	if err != nil {
		return fmt.Errorf("posting login: %w", err)
	}
	resp.Body.Close()

	// pgAdmin redirects on success (302) or returns 200 on the authenticated page
	if resp.StatusCode >= 400 {
		return fmt.Errorf("login failed with status %d", resp.StatusCode)
	}

	// Refresh CSRF token from authenticated session
	a.refreshCSRFToken()

	a.loggedIn = true
	slog.Info("logged into pgAdmin API")
	return nil
}

// refreshCSRFToken fetches a page after login to extract the up-to-date CSRF token.
func (a *APIClient) refreshCSRFToken() {
	resp, err := a.client.Get(a.baseURL + "/browser/")
	if err != nil {
		return
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if token := extractCSRFToken(string(body)); token != "" {
		a.csrfToken = token
	}
}

// extractCSRFToken finds the CSRF token from an HTML page body.
func extractCSRFToken(body string) string {
	// pgAdmin embeds CSRF token in a meta tag or hidden input
	// <input id="csrf_token" name="csrf_token" type="hidden" value="...">
	for _, marker := range []string{
		`name="csrf_token" type="hidden" value="`,
		`id="csrf_token"`,
		`csrfToken" content="`,
	} {
		idx := strings.Index(body, marker)
		if idx < 0 {
			continue
		}
		start := idx + len(marker)
		// find the closing quote
		rest := body[start:]
		end := strings.IndexByte(rest, '"')
		if end > 0 {
			return rest[:end]
		}
	}
	return ""
}

// ensureLoggedIn logs in if not already authenticated.
func (a *APIClient) ensureLoggedIn() error {
	if !a.loggedIn {
		return a.Login()
	}
	return nil
}

// ListServerGroups returns the list of server groups. We need the group ID for "CNPG Clusters".
func (a *APIClient) ListServerGroups() ([]struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}, error) {
	if err := a.ensureLoggedIn(); err != nil {
		return nil, err
	}

	resp, err := a.doRequest("GET", "/browser/server_group/obj/", nil)
	if err != nil {
		return nil, fmt.Errorf("listing server groups: %w", err)
	}
	defer resp.Body.Close()

	var result []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding server groups: %w", err)
	}
	return result, nil
}

// FindOrCreateServerGroup finds a server group by name or creates it. Returns the group ID.
func (a *APIClient) FindOrCreateServerGroup(name string) (int, error) {
	groups, err := a.ListServerGroups()
	if err != nil {
		return 0, err
	}

	for _, g := range groups {
		if g.Name == name {
			return g.ID, nil
		}
	}

	// Create the group
	payload := map[string]string{"name": name}
	body, _ := json.Marshal(payload)

	resp, err := a.doRequest("POST", "/browser/server_group/obj/", bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("creating server group: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Node struct {
			ID int `json:"_id"`
		} `json:"node"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("decoding server group creation response: %w", err)
	}

	slog.Info("created server group", "name", name, "id", result.Node.ID)
	return result.Node.ID, nil
}

// ListServers returns all servers in a given group.
func (a *APIClient) ListServers(groupID int) ([]APIServer, error) {
	if err := a.ensureLoggedIn(); err != nil {
		return nil, err
	}

	path := fmt.Sprintf("/browser/server/nodes/%d/", groupID)
	resp, err := a.doRequest("GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("listing servers: %w", err)
	}
	defer resp.Body.Close()

	var result []APIServer
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		// pgAdmin might return {"data": [...]} or just [...]
		return nil, fmt.Errorf("decoding servers: %w", err)
	}
	return result, nil
}

// CreateServer creates a new server in pgAdmin.
func (a *APIClient) CreateServer(groupID int, entry ServerEntry) error {
	if err := a.ensureLoggedIn(); err != nil {
		return err
	}

	payload := map[string]interface{}{
		"name":            entry.Name,
		"host":            entry.Host,
		"port":            entry.Port,
		"maintenance_db":  entry.MaintenanceDB,
		"username":        entry.Username,
		"ssl_mode":        entry.SSLMode,
		"comment":         entry.Comment,
		"connect_timeout": 10,
	}
	body, _ := json.Marshal(payload)

	path := fmt.Sprintf("/browser/server/obj/%d/", groupID)
	resp, err := a.doRequest("POST", path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating server %q: %w", entry.Name, err)
	}
	resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("creating server %q: status %d", entry.Name, resp.StatusCode)
	}

	slog.Info("created server in pgAdmin", "name", entry.Name)
	return nil
}

// DeleteServer deletes a server by ID.
func (a *APIClient) DeleteServer(groupID, serverID int) error {
	if err := a.ensureLoggedIn(); err != nil {
		return err
	}

	path := fmt.Sprintf("/browser/server/obj/%d/%d", groupID, serverID)
	resp, err := a.doRequest("DELETE", path, nil)
	if err != nil {
		return fmt.Errorf("deleting server %d: %w", serverID, err)
	}
	resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("deleting server %d: status %d", serverID, resp.StatusCode)
	}

	slog.Info("deleted server from pgAdmin", "id", serverID)
	return nil
}

// doRequest performs an HTTP request with CSRF token and JSON content type.
func (a *APIClient) doRequest(method, path string, body io.Reader) (*http.Response, error) {
	reqURL := a.baseURL + path

	req, err := http.NewRequest(method, reqURL, body)
	if err != nil {
		return nil, err
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if a.csrfToken != "" {
		req.Header.Set("X-CSRFToken", a.csrfToken)
		req.Header.Set("X-pgA-CSRFToken", a.csrfToken)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}

	// If we get a 401/403, try re-authenticating once
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		resp.Body.Close()
		a.loggedIn = false
		if err := a.Login(); err != nil {
			return nil, fmt.Errorf("re-authentication failed: %w", err)
		}
		// Retry the request
		req2, err := http.NewRequest(method, reqURL, body)
		if err != nil {
			return nil, err
		}
		if body != nil {
			req2.Header.Set("Content-Type", "application/json")
		}
		if a.csrfToken != "" {
			req2.Header.Set("X-CSRFToken", a.csrfToken)
			req2.Header.Set("X-pgA-CSRFToken", a.csrfToken)
		}
		return a.client.Do(req2)
	}

	return resp, nil
}

// IsAvailable checks if pgAdmin is reachable.
func (a *APIClient) IsAvailable() bool {
	resp, err := a.client.Get(a.baseURL + "/misc/ping")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}
