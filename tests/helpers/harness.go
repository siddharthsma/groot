package helpers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type HarnessOptions struct {
	AuthMode                    string
	AdminAuthMode               string
	JWKSURL                     string
	BuildEdition                string
	BuildLicensePublicKeyBase64 string
	ExtraEnv                    map[string]string
}

type Harness struct {
	T        *testing.T
	RootDir  string
	BaseURL  string
	AdminKey string
	DB       *sql.DB
	HTTP     *http.Client
	Mocks    *MockSuite

	apiCmd  *exec.Cmd
	tempDir string
	logs    bytes.Buffer
	apiPort int
}

func NewHarness(t *testing.T, opts HarnessOptions) *Harness {
	t.Helper()
	root := repoRoot()
	db, err := sql.Open("pgx", "postgres://groot:groot@localhost:5432/groot?sslmode=disable")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	h := &Harness{
		T:        t,
		RootDir:  root,
		AdminKey: "phase20-admin-secret",
		DB:       db,
		HTTP:     &http.Client{Timeout: 15 * time.Second},
		Mocks:    NewMockSuite(),
	}
	t.Cleanup(h.Mocks.Close)
	if err := h.ResetDatabase(); err != nil {
		t.Fatalf("reset database: %v", err)
	}
	if err := h.StartAPI(opts); err != nil {
		t.Fatalf("start api: %v", err)
	}
	t.Cleanup(func() {
		if err := h.StopAPI(); err != nil {
			t.Fatalf("stop api: %v", err)
		}
	})
	return h
}

func (h *Harness) StartAPI(opts HarnessOptions) error {
	port, err := freePort()
	if err != nil {
		return err
	}
	h.apiPort = port
	h.BaseURL = fmt.Sprintf("http://127.0.0.1:%d", port)
	authMode := strings.TrimSpace(opts.AuthMode)
	if authMode == "" {
		authMode = "api_key"
	}
	adminAuthMode := strings.TrimSpace(opts.AdminAuthMode)
	if adminAuthMode == "" {
		adminAuthMode = "api_key"
	}
	runtimeEdition := "internal"
	if edition := strings.TrimSpace(opts.BuildEdition); edition != "" {
		runtimeEdition = edition
	}
	if override, ok := opts.ExtraEnv["GROOT_EDITION"]; ok && strings.TrimSpace(override) != "" {
		runtimeEdition = override
	}
	env := append(os.Environ(),
		"GROOT_EDITION="+runtimeEdition,
		"GROOT_HTTP_ADDR=127.0.0.1:"+fmt.Sprint(port),
		"POSTGRES_DSN=postgres://groot:groot@localhost:5432/groot?sslmode=disable",
		"KAFKA_BROKERS=localhost:9092",
		"ROUTER_CONSUMER_GROUP=phase20-router-"+fmt.Sprint(port),
		"TEMPORAL_ADDRESS=localhost:7233",
		"TEMPORAL_NAMESPACE=default",
		"GROOT_DELIVERY_TASK_QUEUE=phase20-delivery-"+fmt.Sprint(port),
		"GROOT_SYSTEM_API_KEY=system-secret",
		"AUTH_MODE="+authMode,
		"API_KEY_HEADER=X-API-Key",
		"TENANT_HEADER=X-Tenant-Id",
		"ACTOR_ID_HEADER=X-Actor-Id",
		"ACTOR_TYPE_HEADER=X-Actor-Type",
		"ACTOR_EMAIL_HEADER=X-Actor-Email",
		"JWT_AUDIENCE=groot",
		"JWT_ISSUER=https://phase20.local/",
		"JWT_REQUIRED_CLAIMS=sub,tenant_id",
		"JWT_TENANT_CLAIM=tenant_id",
		"JWT_CLOCK_SKEW_SECONDS=60",
		"ADMIN_MODE_ENABLED=true",
		"ADMIN_AUTH_MODE="+adminAuthMode,
		"ADMIN_API_KEY="+h.AdminKey,
		"ADMIN_JWT_AUDIENCE=groot-admin",
		"ADMIN_JWT_ISSUER=https://phase20.local/",
		"ADMIN_JWT_REQUIRED_CLAIMS=sub",
		"ADMIN_ALLOW_VIEW_PAYLOADS=false",
		"ADMIN_REPLAY_ENABLED=true",
		"ADMIN_REPLAY_MAX_EVENTS=100",
		"ADMIN_RATE_LIMIT_RPS=100",
		"AUDIT_ENABLED=true",
		"GROOT_ALLOW_GLOBAL_INSTANCES=true",
		"MAX_CHAIN_DEPTH=10",
		"MAX_REPLAY_EVENTS=1000",
		"MAX_REPLAY_WINDOW_HOURS=24",
		"SCHEMA_VALIDATION_MODE=reject",
		"SCHEMA_REGISTRATION_MODE=startup",
		"SCHEMA_MAX_PAYLOAD_BYTES=262144",
		"GRAPH_MAX_NODES=5000",
		"GRAPH_MAX_EDGES=20000",
		"GRAPH_EXECUTION_TRAVERSAL_MAX_EVENTS=500",
		"GRAPH_EXECUTION_MAX_DEPTH=25",
		"GRAPH_DEFAULT_LIMIT=500",
		"STRIPE_WEBHOOK_TOLERANCE_SECONDS=300",
		"RESEND_API_KEY=phase20-resend-api-key",
		"RESEND_API_BASE_URL="+h.Mocks.ResendAPIServer.URL,
		"RESEND_WEBHOOK_PUBLIC_URL="+h.BaseURL+"/webhooks/resend",
		"RESEND_RECEIVING_DOMAIN=phase20.resend.test",
		"RESEND_WEBHOOK_EVENTS=email.received",
		"SLACK_API_BASE_URL="+h.Mocks.SlackServer.URL,
		"SLACK_SIGNING_SECRET="+h.Mocks.SlackSigningSecret,
		"NOTION_API_BASE_URL="+h.Mocks.NotionServer.URL,
		"NOTION_API_VERSION=2022-06-28",
		"OPENAI_API_KEY=phase20-openai-key",
		"OPENAI_API_BASE_URL="+h.Mocks.LLMServer.URL,
		"ANTHROPIC_API_KEY=phase20-anthropic-key",
		"ANTHROPIC_API_BASE_URL="+h.Mocks.LLMServer.URL,
		"LLM_DEFAULT_PROVIDER=openai",
		"LLM_DEFAULT_CLASSIFY_MODEL=gpt-4o-mini",
		"LLM_DEFAULT_EXTRACT_MODEL=gpt-4o-mini",
		"LLM_TIMEOUT_SECONDS=10",
		"AGENT_MAX_STEPS=6",
		"AGENT_STEP_TIMEOUT_SECONDS=15",
		"AGENT_TOTAL_TIMEOUT_SECONDS=60",
		"AGENT_MAX_TOOL_CALLS=6",
		"AGENT_MAX_TOOL_OUTPUT_BYTES=16384",
		"AGENT_RUNTIME_ENABLED=true",
		"AGENT_RUNTIME_BASE_URL=http://127.0.0.1:8090",
		"AGENT_RUNTIME_TIMEOUT_SECONDS=10",
		"AGENT_SESSION_AUTO_CREATE=true",
		"AGENT_SESSION_MAX_IDLE_DAYS=30",
		"AGENT_MEMORY_MODE=runtime_managed",
		"AGENT_MEMORY_SUMMARY_MAX_BYTES=16384",
		"AGENT_RUNTIME_SHARED_SECRET=phase21-runtime-secret",
		"DELIVERY_MAX_ATTEMPTS=3",
		"DELIVERY_INITIAL_INTERVAL=1s",
		"DELIVERY_MAX_INTERVAL=2s",
	)
	if strings.TrimSpace(opts.JWKSURL) != "" {
		env = append(env, "JWT_JWKS_URL="+opts.JWKSURL, "ADMIN_JWT_JWKS_URL="+opts.JWKSURL)
	}
	for key, value := range opts.ExtraEnv {
		env = append(env, key+"="+value)
	}
	tempDir, err := os.MkdirTemp("", "groot-phase20-*")
	if err != nil {
		return err
	}
	h.tempDir = tempDir
	binaryPath := filepath.Join(tempDir, "groot-api")
	buildArgs := []string{"build"}
	ldflags := make([]string, 0, 2)
	if edition := strings.TrimSpace(opts.BuildEdition); edition != "" {
		ldflags = append(ldflags, "-X", "main.BuildEdition="+edition)
	}
	if publicKey := strings.TrimSpace(opts.BuildLicensePublicKeyBase64); publicKey != "" {
		ldflags = append(ldflags, "-X", "main.BuildLicensePublicKeyBase64="+publicKey)
	}
	if len(ldflags) > 0 {
		buildArgs = append(buildArgs, "-ldflags", strings.Join(ldflags, " "))
	}
	buildArgs = append(buildArgs, "-o", binaryPath, "./cmd/groot-api")
	buildCmd := exec.Command("go", buildArgs...)
	buildCmd.Dir = h.RootDir
	buildCmd.Env = env
	buildCmd.Stdout = &h.logs
	buildCmd.Stderr = &h.logs
	if err := buildCmd.Run(); err != nil {
		return err
	}
	cmd := exec.Command(binaryPath)
	cmd.Dir = h.RootDir
	cmd.Env = env
	cmd.Stdout = &h.logs
	cmd.Stderr = &h.logs
	if err := cmd.Start(); err != nil {
		return err
	}
	h.apiCmd = cmd
	if err := h.waitReady(); err != nil {
		_ = h.StopAPI()
		return fmt.Errorf("%w: %s", err, h.Logs())
	}
	return nil
}

func (h *Harness) StopAPI() error {
	if h.apiCmd == nil || h.apiCmd.Process == nil {
		return nil
	}
	done := make(chan error, 1)
	go func() {
		done <- h.apiCmd.Wait()
	}()
	_ = h.apiCmd.Process.Signal(os.Interrupt)
	select {
	case <-time.After(10 * time.Second):
		_ = h.apiCmd.Process.Kill()
		<-done
	case <-done:
	}
	h.apiCmd = nil
	if h.tempDir != "" {
		_ = os.RemoveAll(h.tempDir)
		h.tempDir = ""
	}
	return nil
}

func (h *Harness) ResetDatabase() error {
	return ResetDatabase(h.RootDir, h.DB)
}

func ResetDatabase(root string, db *sql.DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	statements := []string{
		"DROP SCHEMA IF EXISTS public CASCADE",
		"CREATE SCHEMA public",
		"GRANT ALL ON SCHEMA public TO groot",
		"GRANT ALL ON SCHEMA public TO public",
	}
	for _, stmt := range statements {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("exec reset statement %q: %w", stmt, err)
		}
	}
	files, err := filepath.Glob(filepath.Join(root, "migrations", "*.sql"))
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, file := range files {
		body, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", file, err)
		}
		if _, err := db.ExecContext(ctx, string(body)); err != nil {
			return fmt.Errorf("apply migration %s: %w", filepath.Base(file), err)
		}
	}
	return nil
}

func (h *Harness) Logs() string {
	return h.logs.String()
}

func (h *Harness) AssertNoSecrets(values ...string) {
	text := h.Logs()
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if strings.Contains(text, value) {
			h.T.Fatalf("logs contain secret %q\nlogs:\n%s", value, text)
		}
	}
}

func (h *Harness) Request(method, path string, headers http.Header, body io.Reader) (*http.Response, []byte) {
	h.T.Helper()
	req, err := http.NewRequest(method, h.BaseURL+path, body)
	if err != nil {
		h.T.Fatalf("new request %s %s: %v", method, path, err)
	}
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	resp, err := h.HTTP.Do(req)
	if err != nil {
		h.T.Fatalf("do request %s %s: %v\nlogs:\n%s", method, path, err, h.Logs())
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		_ = resp.Body.Close()
		h.T.Fatalf("read response %s %s: %v", method, path, err)
	}
	_ = resp.Body.Close()
	return resp, respBody
}

func (h *Harness) JSONRequest(method, path string, headers http.Header, payload any) (*http.Response, []byte) {
	h.T.Helper()
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			h.T.Fatalf("marshal request: %v", err)
		}
		body = bytes.NewReader(raw)
		if headers == nil {
			headers = make(http.Header)
		}
		headers.Set("Content-Type", "application/json")
	}
	return h.Request(method, path, headers, body)
}

func (h *Harness) CreateTenant(name string) (tenantID string, legacyKey string) {
	h.T.Helper()
	resp, body := h.JSONRequest(http.MethodPost, "/tenants", nil, map[string]any{"name": name})
	if resp.StatusCode != http.StatusOK {
		h.T.Fatalf("create tenant status=%d body=%s", resp.StatusCode, body)
	}
	var payload struct {
		TenantID string `json:"tenant_id"`
		APIKey   string `json:"api_key"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		h.T.Fatalf("decode create tenant: %v", err)
	}
	return payload.TenantID, payload.APIKey
}

func (h *Harness) WaitForEvent(tenantID, eventType string, timeout time.Duration) map[string]any {
	h.T.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if event, ok := h.FindEvent(tenantID, eventType); ok {
			return event
		}
		time.Sleep(250 * time.Millisecond)
	}
	h.T.Fatalf("timed out waiting for event %s", eventType)
	return nil
}

func (h *Harness) FindEvent(tenantID, eventType string) (map[string]any, bool) {
	row := h.DB.QueryRow(`SELECT event_id, type, source, payload::text FROM events WHERE tenant_id = $1 AND type = $2 ORDER BY timestamp DESC LIMIT 1`, tenantID, eventType)
	var id string
	var typ string
	var source string
	var payloadText string
	if err := row.Scan(&id, &typ, &source, &payloadText); err != nil {
		return nil, false
	}
	return map[string]any{"event_id": id, "type": typ, "source": source, "payload": payloadText}, true
}

func (h *Harness) WaitForDeliveryStatus(tenantID string, eventID string, status string, timeout time.Duration) map[string]any {
	h.T.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		row := h.DB.QueryRow(`SELECT id, status, external_id, last_status_code, result_event_id FROM delivery_jobs WHERE tenant_id = $1 AND event_id = $2 ORDER BY created_at DESC LIMIT 1`, tenantID, eventID)
		var id string
		var gotStatus string
		var externalID sql.NullString
		var lastStatus sql.NullInt64
		var resultEventID sql.NullString
		if err := row.Scan(&id, &gotStatus, &externalID, &lastStatus, &resultEventID); err == nil && gotStatus == status {
			return map[string]any{
				"id":               id,
				"status":           gotStatus,
				"external_id":      nullString(externalID),
				"last_status_code": nullInt(lastStatus),
				"result_event_id":  nullString(resultEventID),
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	h.T.Fatalf("timed out waiting for delivery status %s", status)
	return nil
}

func (h *Harness) LatestDeliveryForEvent(tenantID string, eventID string) (map[string]any, bool) {
	row := h.DB.QueryRow(`SELECT id, status, external_id, last_status_code, result_event_id FROM delivery_jobs WHERE tenant_id = $1 AND event_id = $2 ORDER BY created_at DESC LIMIT 1`, tenantID, eventID)
	var id string
	var gotStatus string
	var externalID sql.NullString
	var lastStatus sql.NullInt64
	var resultEventID sql.NullString
	if err := row.Scan(&id, &gotStatus, &externalID, &lastStatus, &resultEventID); err != nil {
		return nil, false
	}
	return map[string]any{
		"id":               id,
		"status":           gotStatus,
		"external_id":      nullString(externalID),
		"last_status_code": nullInt(lastStatus),
		"result_event_id":  nullString(resultEventID),
	}, true
}

func (h *Harness) waitReady() error {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := h.HTTP.Get(h.BaseURL + "/readyz")
		if err == nil {
			_, _ = io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		if h.apiCmd != nil && h.apiCmd.ProcessState != nil && h.apiCmd.ProcessState.Exited() {
			return errors.New("api process exited before ready")
		}
		time.Sleep(500 * time.Millisecond)
	}
	return errors.New("timed out waiting for readyz")
}

func repoRoot() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func RepoRoot() string {
	return repoRoot()
}

func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

func nullString(value sql.NullString) any {
	if value.Valid {
		return value.String
	}
	return nil
}

func nullInt(value sql.NullInt64) any {
	if value.Valid {
		return int(value.Int64)
	}
	return nil
}
