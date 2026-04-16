package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// --- Configuration Structs ---

type SeedConfig struct {
	WsURL         string              `json:"ws_url"`
	FirstNames    []string            `json:"first_names"`
	LastNames     []string            `json:"last_names"`
	Admin         Admin               `json:"admin"`
	TestUsers     []TestUser          `json:"test_users"`
	OrgThemes     []OrgTheme          `json:"org_themes"`
	Agents        []AgentConfig       `json:"agents"`
	QuestionSets  []QuestionSetConfig `json:"question_sets"`
	StartRun      bool                `json:"start_run"`
	RunAgentCount int                 `json:"run_agent_count"`
}

type Admin struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
	OrgName  string `json:"org_name"`
}

type TestUser struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
	OrgName  string `json:"org_name"`
	Role     string `json:"role"`
}

type OrgTheme struct {
	Name     string   `json:"name"`
	Category string   `json:"category"`
	Domains  []string `json:"domains"`
}

type AgentConfig struct {
	Name         string         `json:"name"`
	ProviderType string         `json:"provider_type"`
	Config       map[string]any `json:"config"`
}

type QuestionSetConfig struct {
	Name string `json:"name"`
	Data any    `json:"data"`
}

// WorkspaceInfo stores workspace details for benchmark generation
type WorkspaceInfo struct {
	ID           string
	UserID       string
	OrgID        string
	OrgName      string
	Name         string
	AgentIDs     []string
	QuestionSets []string
}

func getOrgInitials(name string) string {
	parts := strings.Split(name, " ")
	initials := ""
	for _, p := range parts {
		if len(p) > 0 {
			initials += strings.ToUpper(string(p[0]))
		}
	}
	if initials == "" {
		return "WS"
	}
	return initials
}

// titleASCII returns s with its first letter upper-cased and the rest lower-cased.
// Replacement for the deprecated strings.Title — good enough for short ASCII
// role names ("admin", "manager", "viewer") used in seed workspace naming.
func titleASCII(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
}

var MOCK_ANSWERS = []string{
	"The answer is correct according to the logic provided.",
	"Tokyo is indeed the capital city of Japan.",
	"The pills will last exactly one hour (0m, 30m, 60m).",
	"Leonardo da Vinci completed the Mona Lisa.",
	"299,792,458 meters per second in a vacuum.",
	"Compound interest generates exponential growth.",
	"Kubernetes manages containers across clusters.",
	"Yes, following the transitive property (A -> B -> C).",
	"George Orwell wrote 1984 in 1949.",
	"Au comes from the Latin word Aurum.",
	"TCP ensures delivery, UDP sends packets without verification.",
	"Binary search cuts the search space in half each step.",
	"Deep Learning is a subset of Machine Learning using neural networks.",
	"The Trolley Problem highlights utilitarian vs deontological ethics.",
	"Red leaves falling down / Gold and crunch under my feet / Winter is waking.",
	"The Red Bean Roastery.",
	"It smells like wet asphalt and fresh soil.",
	"To measure 4L: Fill 5, pour to 3. 2 left in 5. Empty 3. Pour 2 to 3. Fill 5. Pour 1 to 3. 4 left.",
	"O(n^2) is the worst case for bubble sort.",
	"HTTP is stateless, HTTPS is secure.",
}

// --- WebSocket Client ---

type WSClient struct {
	conn           *websocket.Conn
	responses      map[string]chan Response
	mu             sync.Mutex // Protects responses map and other fields
	writeMu        sync.Mutex // Protects WebSocket writes
	done           chan struct{}
	workspaceID    string
	organizationID string
}

type Response struct {
	Type    string
	Payload json.RawMessage
}

type Envelope struct {
	Type          string          `json:"type"`
	CorrelationID string          `json:"correlation_id"`
	Payload       json.RawMessage `json:"payload"`
}

func NewWSClient(wsURL string) (*WSClient, error) {
	u, err := url.Parse(wsURL)
	if err != nil {
		return nil, err
	}

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("dial error: %w", err)
	}

	client := &WSClient{
		conn:      conn,
		responses: make(map[string]chan Response),
		done:      make(chan struct{}),
	}

	go client.readLoop()
	return client, nil
}

func (c *WSClient) readLoop() {
	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			close(c.done)
			return
		}

		var env Envelope
		if err := json.Unmarshal(msg, &env); err != nil {
			continue
		}

		c.mu.Lock()
		if ch, ok := c.responses[env.CorrelationID]; ok {
			ch <- Response{Type: env.Type, Payload: env.Payload}
			delete(c.responses, env.CorrelationID)
		}
		c.mu.Unlock()
	}
}

func (c *WSClient) Send(msgType string, payload any) (json.RawMessage, error) {
	corrID := uuid.New().String()

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	env := Envelope{
		Type:          msgType,
		CorrelationID: corrID,
		Payload:       payloadBytes,
	}

	envBytes, err := json.Marshal(env)
	if err != nil {
		return nil, err
	}

	respChan := make(chan Response, 1)
	c.mu.Lock()
	c.responses[corrID] = respChan
	c.mu.Unlock()

	c.writeMu.Lock()
	err = c.conn.WriteMessage(websocket.TextMessage, envBytes)
	c.writeMu.Unlock()

	if err != nil {
		c.mu.Lock()
		delete(c.responses, corrID)
		c.mu.Unlock()
		return nil, err
	}

	select {
	case resp := <-respChan:
		if resp.Type == "EVT_ERROR" {
			var errPayload struct {
				Error   string `json:"error"`
				Details any    `json:"details"`
			}
			if err := json.Unmarshal(resp.Payload, &errPayload); err != nil {
				return nil, fmt.Errorf("server returned EVT_ERROR with unparseable payload: %w", err)
			}
			if errPayload.Details != nil {
				return nil, fmt.Errorf("%s (details: %v)", errPayload.Error, errPayload.Details)
			}
			return nil, fmt.Errorf("%s", errPayload.Error)
		}
		return resp.Payload, nil
	case <-time.After(300 * time.Second):
		c.mu.Lock()
		delete(c.responses, corrID)
		c.mu.Unlock()
		return nil, fmt.Errorf("timeout waiting for response")
	case <-c.done:
		return nil, fmt.Errorf("connection closed")
	}
}

func (c *WSClient) SendWithRetry(msgType string, payload any, maxRetries int) (json.RawMessage, error) {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		resp, err := c.Send(msgType, payload)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if i < maxRetries-1 {
			backoff := time.Duration(1<<uint(i)) * time.Second
			log.Printf("  ⚠ [%s] Request failed: %v. Retrying in %v... (%d/%d)", msgType, err, backoff, i+1, maxRetries)
			time.Sleep(backoff)
		}
	}
	return nil, fmt.Errorf("after %d retries: %w", maxRetries, lastErr)
}

func (c *WSClient) Close() {
	c.conn.Close()
}

// --- Helper Functions ---

func generateEmail(firstName, lastName, domain string) string {
	return fmt.Sprintf("%s.%s@%s", strings.ToLower(firstName), strings.ToLower(lastName), domain)
}

func generatePassword() string {
	return fmt.Sprintf("pass%d", rand.Intn(9000)+1000)
}

func domainFromOrgName(orgName string) string {
	// Convert "Quantum Finance" -> "quantumfinance.com"
	clean := strings.ToLower(strings.ReplaceAll(orgName, " ", ""))
	return clean + ".com"
}

func extractQuestionIDs(data any) []string {
	// Best-effort extraction of question IDs from config data.
	seen := make(map[string]bool)
	var ids []string

	var walk func(any)
	walk = func(val any) {
		switch v := val.(type) {
		case map[string]any:
			// Common pattern: map with "questions": [...]
			if q, ok := v["questions"]; ok {
				walk(q)
			}
			if idRaw, ok := v["id"]; ok {
				if idStr, ok := idRaw.(string); ok && idStr != "" && !seen[idStr] {
					seen[idStr] = true
					ids = append(ids, idStr)
				}
			}
		case []any:
			for _, item := range v {
				walk(item)
			}
		}
	}

	walk(data)
	return ids
}

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func ensureDockerNetwork(name string) error {
	inspect := exec.Command("docker", "network", "inspect", name)
	if err := inspect.Run(); err == nil {
		log.Printf("  ✓ Docker network %q already exists", name)
		return nil
	}

	create := exec.Command("docker", "network", "create", name)
	output, err := create.CombinedOutput()
	if err != nil {
		out := strings.TrimSpace(string(output))
		if strings.Contains(out, "already exists") {
			log.Printf("  ✓ Docker network %q already exists", name)
			return nil
		}
		return fmt.Errorf("docker network create %s failed: %v (%s)", name, err, out)
	}

	log.Printf("  ✓ Docker network %q created", name)
	return nil
}

func cleanupNamedContainers(containerNames ...string) {
	for _, name := range containerNames {
		inspect := exec.Command("docker", "container", "inspect", name)
		if err := inspect.Run(); err != nil {
			continue
		}

		log.Printf("  ⚠ Removing stale container %q to avoid compose name conflicts...", name)
		if err := runCommand("docker", "rm", "-f", name); err != nil {
			log.Printf("  ⚠ Failed to remove stale container %q: %v", name, err)
			continue
		}
		log.Printf("  ✓ Removed stale container %q", name)
	}
}

func findProjectRoot() string {
	// Try to find docker-compose.yml by walking up directories
	dir, err := os.Getwd()
	if err != nil {
		log.Printf("  ⚠ os.Getwd failed while searching for project root: %v", err)
		return ""
	}
	for i := 0; i < 5; i++ {
		if _, err := os.Stat(filepath.Join(dir, "docker-compose.yml")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	return ""
}

// --- Seeder Logic ---

func main() {
	configPath := flag.String("config", "seed_config.json", "Path to JSON config file")
	usersPerOrg := flag.Int("users-per-org", 3, "Number of random users to create per org")
	reset := flag.Bool("reset", false, "Reset database only (keeps frontend running)")
	hardReset := flag.Bool("hard-reset", false, "Full reset: docker compose down -v && up -d")
	softReset := flag.Bool("soft-reset", false, "Soft reset: docker compose down && up -d --build (keeps Data)")
	noDb := flag.Bool("no-db", false, "Skip database restart during soft reset")
	live := flag.Bool("live", false, "Live reset: Clears DB, creates Admin, and exits (no mock data)")
	projectDir := flag.String("project-dir", "", "Project directory containing docker-compose.yml")
	flag.Parse()

	log.Println("🚀 Go Reset & Seeder Started")

	// Resolve config path to absolute BEFORE changing directories
	absConfigPath := *configPath
	if !filepath.IsAbs(absConfigPath) {
		cwd, err := os.Getwd()
		if err != nil {
			log.Fatalf("  ❌ Failed to resolve current working directory: %v", err)
		}
		absConfigPath = filepath.Join(cwd, absConfigPath)
	}

	// Find project directory if needed for reset operations
	projDir := *projectDir
	if (*reset || *hardReset || *softReset) && projDir == "" {
		projDir = findProjectRoot()
	}

	if projDir != "" {
		if err := os.Chdir(projDir); err != nil {
			log.Fatalf("  ❌ Failed to chdir to project dir %q: %v", projDir, err)
		}
		if err := ensureDockerNetwork("benchmarking-public"); err != nil {
			log.Printf("  ⚠ Failed to ensure docker network: %v", err)
		}
	}

	// If config path is relative and missing, try resolving from project root.
	if _, err := os.Stat(absConfigPath); err != nil && projDir != "" && !filepath.IsAbs(*configPath) {
		candidate := filepath.Join(projDir, *configPath)
		if _, err := os.Stat(candidate); err == nil {
			absConfigPath = candidate
		}
	}

	// Load config EARLY so we can use it for pre-reset operations
	log.Printf("\n== Loading Config from %s ==", absConfigPath)
	configData, err := os.ReadFile(absConfigPath)
	if err != nil {
		log.Fatalf("  ❌ Failed to read config: %v", err)
	}

	var config SeedConfig
	if err := json.Unmarshal(configData, &config); err != nil {
		log.Fatalf("  ❌ Failed to parse config: %v", err)
	}
	log.Println("  ✓ Config loaded")

	// 0. Pre-Reset: Force Global Logout (if possible)
	if *reset || *hardReset || *softReset || *live {
		log.Println("\n== Pre-Reset: Attempting Global Logout ==")
		// Try to connect to potentially running server
		preClient, err := NewWSClient(config.WsURL)
		if err == nil {
			// Login as Admin
			log.Println("  ⏳ Connected, logging in to force logout...")
			resp, err := preClient.Send("REQ_WS_LOGIN", map[string]string{
				"email":    config.Admin.Email,
				"password": config.Admin.Password,
			})

			if err == nil {
				var loginResult struct {
					Success bool `json:"success"`
				}
				if jerr := json.Unmarshal(resp, &loginResult); jerr != nil {
					log.Printf("  ⚠ Failed to parse pre-login response: %v", jerr)
				}

				if loginResult.Success {
					log.Println("  ✓ Logged in. Sending Maintenance Signal...")
					// 1. Broadcast Maintenance Mode Start
					_, err = preClient.Send("CMD_ADMIN_START_MAINTENANCE", map[string]string{})
					if err == nil {
						log.Println("  ✓ Maintenance signal sent. Waiting 2s for UI transition...")
						time.Sleep(2 * time.Second)
					} else {
						log.Printf("  ⚠ Failed to send maintenance command: %v", err)
					}

					log.Println("  INFO: Skipping force logout as maintenance mode handles user redirection.")
					// Alternatively, we could still force logout if we wanted to be sure
					// _, err = preClient.Send("CMD_ADMIN_FORCE_LOGOUT", map[string]string{})
				} else {
					log.Println("  ⚠ Login failed (maybe DB is already stale), skipping logout.")
				}
			} else {
				log.Printf("  ⚠ Login request failed: %v", err)
			}
			preClient.Close()
		} else {
			log.Println("  ⚠ Could not connect to existing server (it might be down), skipping logout.")
		}
	}

	// Soft reset: docker compose down (preserves volumes) && up -d --build
	if *softReset {
		log.Println("\n== Soft Reset (containers rebuild, data preserved) ==")
		staleAppContainers := []string{
			"AgenteEval-api",
			"benchmarking-db",
			"AgenteEval-frontend",
			"AgenteEval-frontend-dev",
			"python-runner",
		}
		staleNoDbContainers := []string{
			"AgenteEval-api",
			"AgenteEval-frontend",
			"AgenteEval-frontend-dev",
			"python-runner",
		}

		if projDir == "" {
			log.Fatalf("  ❌ Cannot find docker-compose.yml. Use --project-dir flag.")
		}

		log.Printf("  📁 Project dir: %s", projDir)

		if *noDb {
			log.Println("  ⏳ Stopping app containers EXCEPT DB...")
			// Create list of services to restart (excluding db)
			services := []string{"go-api", "frontend-dev", "frontend"}

			// We stop them specifically instead of 'down'
			args := append([]string{"compose", "stop"}, services...)
			if err := runCommand("docker", args...); err != nil {
				log.Printf("  ⚠ Warning: failed to stop services: %v", err)
			}

			log.Println("  ⏳ Starting Services (excluding db)...")
			cleanupNamedContainers(staleNoDbContainers...)
			// We explicitly bring up the non-db services
			// Note: We use the same services list as stop + ensure we target what we need
			upArgs := []string{"compose", "up", "-d", "--build", "go-api", "frontend-dev"}
			if err := runCommand("docker", upArgs...); err != nil {
				log.Fatalf("  ❌ docker compose up (services) failed: %v", err)
			}
		} else {
			log.Println("  ⏳ Stopping app containers...")
			if err := runCommand("docker", "compose", "down", "--remove-orphans"); err != nil {
				log.Fatalf("  ❌ docker compose down failed: %v", err)
			}
			log.Println("  ✓ Application services stopped")

			log.Println("  ⏳ Starting Backend (db, api)...")
			cleanupNamedContainers(staleAppContainers...)
			if err := runCommand("docker", "compose", "up", "-d", "--build", "db", "go-api"); err != nil {
				log.Fatalf("  ❌ docker compose up (backend) failed: %v", err)
			}
			log.Println("  ✓ Backend started")

			log.Println("  ⏳ Starting Frontend...")
			if err := runCommand("docker", "compose", "up", "-d", "--build", "frontend-dev"); err != nil {
				log.Fatalf("  ❌ docker compose up (frontend) failed: %v", err)
			}
		}

		log.Println("  ✓ Services started")
		log.Println("  ✅ Soft reset complete. Exiting without seeding.")
		return
	}

	// Hard reset: docker compose down -v && up -d (all containers)
	if *hardReset {
		log.Println("\n== Hard Reset (all application services) ==")
		staleAppContainers := []string{
			"AgenteEval-api",
			"benchmarking-db",
			"AgenteEval-frontend",
			"AgenteEval-frontend-dev",
			"python-runner",
		}

		if projDir == "" {
			log.Fatalf("  ❌ Cannot find docker-compose.yml. Use --project-dir flag.")
		}

		log.Printf("  📁 Project dir: %s", projDir)

		log.Println("  ⏳ Stopping app containers and removing volumes...")
		if err := runCommand("docker", "compose", "down", "-v", "--remove-orphans"); err != nil {
			log.Fatalf("  ❌ docker compose down failed: %v", err)
		}
		log.Println("  ✓ App services stopped, volumes removed")

		log.Println("  ⏳ Starting Backend (db, api)...")
		cleanupNamedContainers(staleAppContainers...)
		if err := runCommand("docker", "compose", "up", "-d", "--build", "db", "go-api"); err != nil {
			log.Fatalf("  ❌ docker compose up (backend) failed: %v", err)
		}
		log.Println("  ✓ Backend started")

		log.Println("  ⏳ Starting Frontend...")
		if err := runCommand("docker", "compose", "up", "-d", "--build", "frontend-dev"); err != nil {
			log.Fatalf("  ❌ docker compose up (frontend) failed: %v", err)
		}
		log.Println("  ✓ Frontend started")

		log.Println("  ⏳ Waiting 3s for services to initialize...")
		time.Sleep(3 * time.Second)
	}

	// Soft reset (DB only): default --reset behavior, OR if --live is set
	if (*reset && !*hardReset) || *live {
		log.Println("\n== Resetting Database Only ==")

		if projDir == "" {
			log.Fatalf("  ❌ Cannot find docker-compose.yml. Use --project-dir flag.")
		}

		if err := os.Chdir(projDir); err != nil {
			log.Fatalf("  ❌ Failed to chdir to project dir %q: %v", projDir, err)
		}
		log.Printf("  📁 Project dir: %s", projDir)

		log.Println("  ⏳ Stopping db and go-api containers...")
		if err := runCommand("docker", "compose", "stop", "db", "go-api"); err != nil {
			log.Fatalf("  ❌ docker compose stop failed: %v", err)
		}

		log.Println("  ⏳ Removing db container and volume...")
		if err := runCommand("docker", "compose", "rm", "-f", "-s", "db"); err != nil {
			log.Fatalf("  ❌ docker compose rm db failed: %v", err)
		}
		if err := runCommand("docker", "volume", "rm", "-f", "agentcomparison_postgres_data"); err != nil {
			log.Fatalf("  ❌ docker volume rm failed: %v", err)
		}

		log.Println("  ⏳ Restarting db and go-api...")
		if err := runCommand("docker", "compose", "up", "-d", "db", "go-api"); err != nil {
			log.Fatalf("  ❌ docker compose up failed: %v", err)
		}
		log.Println("  ✓ Database reset complete")

		log.Println("  ⏳ Waiting 5s for database to initialize...")
		time.Sleep(5 * time.Second)
	}

	// 1. Wait for API to be available
	log.Println("\n== Waiting for WebSocket API (Post-Reset) ==")
	var client *WSClient
	maxRetries := 30
	for i := 0; i < maxRetries; i++ {
		client, err = NewWSClient(config.WsURL)
		if err == nil {
			break
		}
		log.Printf("  ⏳ API not ready, retrying in 2s... (%d/%d)", i+1, maxRetries)
		time.Sleep(2 * time.Second)
	}
	if client == nil {
		log.Fatalf("  ❌ Failed to connect after %d retries", maxRetries)
	}
	defer client.Close()
	log.Println("  ✓ Connected to WebSocket")

	// 2. Check if Admin Exists
	log.Println("\n== Checking Admin Status ==")
	resp, err := client.Send("REQ_CHECK_ADMIN_EXISTS", nil)
	if err != nil {
		log.Fatalf("  ❌ Failed to check admin: %v", err)
	}

	var adminCheck struct {
		Exists bool `json:"exists"`
	}
	if err := json.Unmarshal(resp, &adminCheck); err != nil {
		log.Fatalf("  ❌ Failed to parse admin-check response: %v", err)
	}

	if !adminCheck.Exists {
		// 3. Bootstrap Admin
		log.Println("\n== Bootstrapping Admin ==")
		_, err = client.SendWithRetry("REQ_WS_BOOTSTRAP_ADMIN", map[string]string{
			"name":              config.Admin.Name,
			"email":             config.Admin.Email,
			"password":          config.Admin.Password,
			"organization_name": config.Admin.OrgName,
		}, 3)
		if err != nil {
			log.Fatalf("  ❌ Failed to bootstrap admin: %v", err)
		}
		log.Printf("  ✓ Admin %s created", config.Admin.Email)
	} else {
		log.Println("  ⚠ Admin already exists, skipping bootstrap")
	}

	if *live {
		log.Println("\n✅ Live Reset Complete! Environment is clean with only the Admin user.")
		return
	}

	// 3.5 Check Database Performance
	log.Println("\n== Checking Database Performance ==")
	perfResp, err := client.Send("REQ_CHECK_DB_PERF", nil)
	if err == nil {
		var perf struct {
			Duration int64 `json:"duration_ms"`
		}
		if jerr := json.Unmarshal(perfResp, &perf); jerr != nil {
			log.Printf("  ⚠ Failed to parse DB perf response: %v", jerr)
		}
		log.Printf("  ✓ DB Latency: %dms", perf.Duration)
		if perf.Duration > 500 {
			log.Println("  ⚠ WARNING: Database is extremely slow (>500ms). Seeding may timeout.")
		}
	} else {
		log.Printf("  ⚠ Failed to check DB performance: %v", err)
	}

	// 4. Login as Admin
	log.Println("\n== Logging in as Admin ==")
	resp, err = client.SendWithRetry("REQ_WS_LOGIN", map[string]string{
		"email":    config.Admin.Email,
		"password": config.Admin.Password,
	}, 3)
	if err != nil {
		log.Fatalf("  ❌ Login failed: %v", err)
	}

	var loginResult struct {
		Success   bool   `json:"success"`
		Token     string `json:"token"`
		Workspace struct {
			ID string `json:"id"`
		} `json:"workspace"`
		Organization struct {
			ID string `json:"id"`
		} `json:"organization"`
	}
	if err := json.Unmarshal(resp, &loginResult); err != nil {
		log.Fatalf("  ❌ Failed to parse login response: %v", err)
	}

	if !loginResult.Success {
		log.Fatalf("  ❌ Login failed: token not returned")
	}

	client.workspaceID = loginResult.Workspace.ID
	client.organizationID = loginResult.Organization.ID
	log.Printf("  ✓ Logged in. Workspace: %s", client.workspaceID)

	// Note: We don't force logout here because we just reset the DB and this is the first login.

	// Track all workspaces for benchmark generation
	allWorkspaces := []WorkspaceInfo{
		{ID: client.workspaceID, OrgID: client.organizationID, Name: "Admin Workspace"},
	}

	// Cache of org names to IDs
	orgCache := map[string]string{
		config.Admin.OrgName: client.organizationID,
	}
	var orgMu sync.Mutex
	var workspaceMu sync.Mutex

	// Helper to get or create org
	getOrCreateOrg := func(orgName string) string {
		orgMu.Lock()
		defer orgMu.Unlock()

		if id, ok := orgCache[orgName]; ok {
			return id
		}

		resp, err := client.SendWithRetry("REQ_ADMIN_CREATE_ORG", map[string]string{
			"name": orgName,
		}, 3)
		if err != nil {
			log.Printf("  ⚠ Failed to create org %s: %v", orgName, err)
			return ""
		}
		var orgResult struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(resp, &orgResult); err != nil {
			log.Printf("  ⚠ Failed to parse org-create response for %s: %v", orgName, err)
			return ""
		}

		orgCache[orgName] = orgResult.ID
		log.Printf("  ✓ Created Org: %s (%s)", orgName, orgResult.ID)
		return orgResult.ID
	}

	// 5. Create Test Users (fixed users from config)
	log.Println("\n== Creating Test Users ==")
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4) // Allow modest concurrency to avoid long serial runs

	for _, user := range config.TestUsers {
		// Skip admin - already created
		if user.Email == config.Admin.Email {
			continue
		}

		wg.Add(1)
		go func(user TestUser) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			orgID := client.organizationID
			if user.OrgName != "" && user.OrgName != config.Admin.OrgName {
				orgID = getOrCreateOrg(user.OrgName)
				if orgID == "" {
					return
				}
			}

			wsName := getOrgInitials(user.OrgName) + " - " + titleASCII(user.Role) + " Space"
			resp, err := client.SendWithRetry("REQ_ADMIN_CREATE_USER", map[string]any{
				"name":            user.Name,
				"email":           user.Email,
				"password":        user.Password,
				"organization_id": orgID,
				"role":            user.Role,
				"is_admin":        user.Role == "admin",
				"workspace_name":  wsName,
			}, 3)
			if err != nil {
				log.Printf("    ⚠ Failed to create %s: %v", user.Email, err)
				return
			}

			var userResult struct {
				ID        string `json:"id"`
				Workspace struct {
					ID string `json:"id"`
				} `json:"workspace"`
			}
			if err := json.Unmarshal(resp, &userResult); err != nil {
				log.Printf("    ⚠ Failed to parse user-create response for %s: %v", user.Email, err)
				return
			}

			if userResult.Workspace.ID != "" {
				workspaceMu.Lock()
				allWorkspaces = append(allWorkspaces, WorkspaceInfo{
					ID:      userResult.Workspace.ID,
					UserID:  userResult.ID,
					OrgID:   orgID,
					OrgName: user.OrgName,
					Name:    wsName,
				})
				workspaceMu.Unlock()
			}

			role := user.Role
			if role == "" {
				role = "member"
			}

			// If this is a manager, explicitly update the org's manager_id
			if role == "manager" && orgID != "" {
				log.Printf("    ⏳ Setting %s as manager for org %s...", user.Email, user.OrgName)
				_, err = client.Send("REQ_ADMIN_UPDATE_ORG", map[string]any{
					"id":         orgID,
					"manager_id": userResult.ID,
				})
				if err != nil {
					log.Printf("    ❌ Failed to set manager for org %s: %v", user.OrgName, err)
				} else {
					log.Printf("    ✅ %s is now Manager of %s", user.Name, user.OrgName)
				}
			}

			log.Printf("  ✓ Created: %s (%s @ %s)", user.Email, role, user.OrgName)
		}(user)
	}
	wg.Wait()

	// 6. Generate Random Orgs and Users (from org_themes)
	if len(config.OrgThemes) > 0 && len(config.FirstNames) > 0 && len(config.LastNames) > 0 {
		log.Println("\n== Generating Random Orgs & Users ==")

		for i := range config.OrgThemes {
			theme := config.OrgThemes[i]

			wg.Add(1)
			go func(theme OrgTheme) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				// Skip if org already exists (from test_users)
				orgMu.Lock()
				_, exists := orgCache[theme.Name]
				orgMu.Unlock()
				if exists {
					return
				}

				// Create the org
				orgID := getOrCreateOrg(theme.Name)
				if orgID == "" {
					return
				}

				// Create random users for this org
				domain := domainFromOrgName(theme.Name)
				for i := 0; i < *usersPerOrg; i++ {
					firstName := config.FirstNames[rand.Intn(len(config.FirstNames))]
					lastName := config.LastNames[rand.Intn(len(config.LastNames))]
					email := generateEmail(firstName, lastName, domain)
					password := generatePassword()
					fullName := firstName + " " + lastName

					// Make the first random user the manager
					role := "member"
					if i == 0 {
						role = "manager"
					}

					wsName := getOrgInitials(theme.Name) + " - " + titleASCII(role) + " Space"
					resp, err := client.SendWithRetry("REQ_ADMIN_CREATE_USER", map[string]any{
						"name":  fullName,
						"email": email,
						"password": func() string {
							if role == "manager" {
								return "password123"
							}
							return password
						}(),
						"organization_id": orgID,
						"role":            role,
						"is_admin":        role == "admin",
						"workspace_name":  wsName,
					}, 3)
					if err != nil {
						log.Printf("    ⚠ Failed to create %s: %v", email, err)
						continue
					}

					var userResult struct {
						ID        string `json:"id"`
						Workspace struct {
							ID string `json:"id"`
						} `json:"workspace"`
					}
					if err := json.Unmarshal(resp, &userResult); err != nil {
						log.Printf("    ⚠ Failed to parse user-create response for %s: %v", email, err)
						continue
					}

					if userResult.Workspace.ID != "" {
						workspaceMu.Lock()
						allWorkspaces = append(allWorkspaces, WorkspaceInfo{
							ID:      userResult.Workspace.ID,
							UserID:  userResult.ID,
							OrgID:   orgID,
							OrgName: theme.Name,
							Name:    wsName,
						})
						workspaceMu.Unlock()
					}

					// Update org manager_id if it's the first user
					if i == 0 {
						log.Printf("    ⏳ Setting %s as manager for org %s...", fullName, theme.Name)
						if _, err := client.Send("REQ_ADMIN_UPDATE_ORG", map[string]any{
							"id":         orgID,
							"manager_id": userResult.ID,
						}); err != nil {
							log.Printf("    ⚠ Failed to set %s as manager of %s: %v", fullName, theme.Name, err)
						} else {
							log.Printf("    ✅ %s is now Manager of %s", fullName, theme.Name)
						}
					} else {
						log.Printf("    ✓ %s (%s)", fullName, email)
					}
				}
			}(theme)
		}
	}
	wg.Wait()

	// 7. Generate Benchmarks for ALL Workspaces
	log.Printf("\n== Generating Benchmarks for %d Workspaces (Concurrent) ==", len(allWorkspaces))

	for i := range allWorkspaces {
		ws := &allWorkspaces[i]
		wg.Add(1)
		go func(ws *WorkspaceInfo) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			// log.Printf("\n  📁 Workspace: %s", ws.Name)
			defaultQIDs := []string{"q1", "q2", "LOG-01", "LOG-02", "GK-01", "CODE-01", "ETH-01"}
			qIDsFromConfig := defaultQIDs
			if len(config.QuestionSets) > 0 {
				if ids := extractQuestionIDs(config.QuestionSets[0].Data); len(ids) > 0 {
					qIDsFromConfig = ids
				}
			}

			// Create Agents for this workspace
			for _, agent := range config.Agents {
				resp, err := client.SendWithRetry("REQ_CREATE_AGENT", map[string]any{
					"workspace_id":  ws.ID,
					"name":          agent.Name,
					"provider_type": agent.ProviderType,
					"config":        agent.Config,
				}, 3)
				if err != nil {
					log.Printf("    ⚠ Failed to create agent %s in %s: %v", agent.Name, ws.Name, err)
					continue
				}
				var agentResult struct {
					ID string `json:"id"`
				}
				if jerr := json.Unmarshal(resp, &agentResult); jerr != nil {
					log.Printf("    ⚠ Failed to parse agent-create response for %s in %s: %v", agent.Name, ws.Name, jerr)
					continue
				}
				ws.AgentIDs = append(ws.AgentIDs, agentResult.ID)
			}

			// Create Question Sets for this workspace
			for _, qs := range config.QuestionSets {
				qsName := getOrgInitials(ws.OrgName) + " " + qs.Name
				if ws.OrgName == "" {
					qsName = qs.Name
				}
				resp, err := client.SendWithRetry("REQ_CREATE_QUESTION_SET", map[string]any{
					"workspace_id": ws.ID,
					"name":         qsName,
					"data":         qs.Data,
				}, 3)
				if err != nil {
					log.Printf("    ⚠ Failed to create question set %s in %s: %v", qs.Name, ws.Name, err)
					continue
				}
				var qsResult struct {
					ID string `json:"id"`
				}
				if jerr := json.Unmarshal(resp, &qsResult); jerr != nil {
					log.Printf("    ⚠ Failed to parse question-set-create response for %s in %s: %v", qs.Name, ws.Name, jerr)
					continue
				}
				ws.QuestionSets = append(ws.QuestionSets, qsResult.ID)

				// LINK AGENTS TO QUESTION SET WITH POSITIONS
				// This populates the question_set_agents table and sets positions
				var agentsPayload []map[string]any
				for pos, agentID := range ws.AgentIDs {
					agentsPayload = append(agentsPayload, map[string]any{
						"agent_id": agentID,
						"config":   map[string]any{},
						"enabled":  true,
						"position": pos, // Explicit position 0, 1, 2...
					})
				}

				_, err = client.SendWithRetry("REQ_UPDATE_QUESTION_SET_AGENTS", map[string]any{
					"question_set_id": qsResult.ID,
					"agents":          agentsPayload,
				}, 3)
				if err != nil {
					log.Printf("    ⚠ Failed to link agents to question set %s: %v", qs.Name, err)
				}
			}

			// GENERATE HISTORICAL RUNS FOR THIS WORKSPACE
			if len(ws.AgentIDs) > 0 && len(ws.QuestionSets) > 0 {
				numRuns := rand.Intn(6) + 3 // 3-8 runs
				// log.Printf("    ⏳ [%s] Seeding %d historical runs...", ws.Name, numRuns)

				for r := 0; r < numRuns; r++ {
					// Random time in last 180 days
					daysAgo := rand.Intn(180)
					createdAt := time.Now().AddDate(0, 0, -daysAgo)

					// Status: mostly completed
					status := "success"
					if rand.Float32() < 0.03 {
						status = "error"
					}

					// Build results payload
					var results []map[string]any

					for _, aID := range ws.AgentIDs {
						for _, qID := range qIDsFromConfig {
							answer := MOCK_ANSWERS[rand.Intn(len(MOCK_ANSWERS))]
							res := map[string]any{
								"agent_id":    aID,
								"question_id": qID,
								"status":      status,
								"answer":      answer,
								"duration_ms": rand.Intn(7500) + 500,
							}

							if status == "success" {
								rating := "like"
								ratingCode := 1
								score := 100
								r := rand.Float32()
								if r < 0.1 {
									rating = "wrong"
									ratingCode = 4
									score = 0
								} else if r < 0.2 {
									rating = "dislike"
									ratingCode = 3
									score = 25
								} else if r < 0.4 {
									rating = "valid"
									ratingCode = 2
									score = 75
								}

								res["evaluations"] = []map[string]any{
									{
										"rater_type":  "user",
										"rating":      rating,
										"rating_code": ratingCode,
										"score":       score,
										"comments":    "Generated by seeder",
									},
								}
							}
							results = append(results, res)
						}
					}

					_, err := client.SendWithRetry("CMD_SEED_HISTORICAL_RUN", map[string]any{
						"workspace_id":    ws.ID,
						"question_set_id": ws.QuestionSets[0],
						"agent_ids":       ws.AgentIDs,
						"created_at":      createdAt,
						"results":         results,
					}, 3)
					if err != nil {
						log.Printf("      ❌ [%s] Failed to seed run: %v", ws.Name, err)
					}
				}
			}
			log.Printf("  ✅ Finished Workspace: %s", ws.Name)
		}(ws)
	}
	wg.Wait()

	// 8. Recalculate Stats (Admin)
	log.Println("\n== Recalculating Stats via WS ==")
	_, err = client.Send("CMD_ADMIN_RECALCULATE_STATS", nil)
	if err != nil {
		log.Printf("  ⚠ Failed to recalculate stats: %v", err)
	} else {
		log.Println("  ✓ Stats recalculation triggered")
	}

	// Summary
	// Summary
	totalAgents := 0
	totalQs := 0
	for _, ws := range allWorkspaces {
		totalAgents += len(ws.AgentIDs)
		totalQs += len(ws.QuestionSets)
	}

	log.Println("\n" + strings.Repeat("=", 50))
	log.Printf("✅ Seeding Complete!")
	log.Printf("   Organizations: %d", len(orgCache))
	log.Printf("   Workspaces: %d", len(allWorkspaces))
	log.Printf("   Agents: %d", totalAgents)
	log.Printf("   Question Sets: %d", totalQs)
	log.Println(strings.Repeat("=", 50))
}
