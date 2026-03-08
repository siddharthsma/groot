package helpers

import (
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	svix "github.com/svix/svix-webhooks/go"
)

type MockSuite struct {
	SlackSigningSecret  string
	ResendSigningSecret string

	slackMu          sync.Mutex
	slackMessages    []map[string]any
	slackInboundSeen []map[string]any

	notionMu       sync.Mutex
	notionRequests []mockRequest

	resendMu       sync.Mutex
	resendEmails   []map[string]any
	resendWebhooks []map[string]any

	llmMu      sync.Mutex
	llmQueue   []string
	llmPrompts []string

	functionMu    sync.Mutex
	functionCalls []mockRequest

	jwksPrivateKey *rsa.PrivateKey
	jwksKeyID      string

	SlackServer     *httptest.Server
	NotionServer    *httptest.Server
	ResendAPIServer *httptest.Server
	LLMServer       *httptest.Server
	FunctionServer  *httptest.Server
	JWKSServer      *httptest.Server
}

type mockRequest struct {
	Method string
	Path   string
	Header http.Header
	Body   []byte
}

func NewMockSuite() *MockSuite {
	suite := &MockSuite{
		SlackSigningSecret:  "phase20-slack-signing-secret",
		ResendSigningSecret: "whsec_" + base64.StdEncoding.EncodeToString([]byte("phase20-resend-secret")),
		jwksKeyID:           "phase20-jwks-key",
	}
	suite.initKeys()
	suite.SlackServer = httptest.NewServer(http.HandlerFunc(suite.handleSlack))
	suite.NotionServer = httptest.NewServer(http.HandlerFunc(suite.handleNotion))
	suite.ResendAPIServer = httptest.NewServer(http.HandlerFunc(suite.handleResendAPI))
	suite.LLMServer = httptest.NewServer(http.HandlerFunc(suite.handleLLM))
	suite.FunctionServer = httptest.NewServer(http.HandlerFunc(suite.handleFunction))
	suite.JWKSServer = httptest.NewServer(http.HandlerFunc(suite.handleJWKS))
	return suite
}

func (m *MockSuite) Close() {
	if m.SlackServer != nil {
		m.SlackServer.Close()
	}
	if m.NotionServer != nil {
		m.NotionServer.Close()
	}
	if m.ResendAPIServer != nil {
		m.ResendAPIServer.Close()
	}
	if m.LLMServer != nil {
		m.LLMServer.Close()
	}
	if m.FunctionServer != nil {
		m.FunctionServer.Close()
	}
	if m.JWKSServer != nil {
		m.JWKSServer.Close()
	}
}

func (m *MockSuite) QueueLLMResponses(responses ...string) {
	m.llmMu.Lock()
	defer m.llmMu.Unlock()
	m.llmQueue = append(m.llmQueue, responses...)
}

func (m *MockSuite) SlackMessages() []map[string]any {
	m.slackMu.Lock()
	defer m.slackMu.Unlock()
	return cloneMapSlice(m.slackMessages)
}

func (m *MockSuite) NotionRequests() []mockRequest {
	m.notionMu.Lock()
	defer m.notionMu.Unlock()
	return append([]mockRequest(nil), m.notionRequests...)
}

func (m *MockSuite) ResendEmails() []map[string]any {
	m.resendMu.Lock()
	defer m.resendMu.Unlock()
	return cloneMapSlice(m.resendEmails)
}

func (m *MockSuite) FunctionCalls() []mockRequest {
	m.functionMu.Lock()
	defer m.functionMu.Unlock()
	return append([]mockRequest(nil), m.functionCalls...)
}

func (m *MockSuite) SignSlack(body []byte, ts time.Time) http.Header {
	timestamp := strconv.FormatInt(ts.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(m.SlackSigningSecret))
	_, _ = mac.Write([]byte("v0:" + timestamp + ":"))
	_, _ = mac.Write(body)
	header := make(http.Header)
	header.Set("X-Slack-Request-Timestamp", timestamp)
	header.Set("X-Slack-Signature", "v0="+hex.EncodeToString(mac.Sum(nil)))
	header.Set("Content-Type", "application/json")
	return header
}

func (m *MockSuite) SignStripe(body []byte, secret string, ts time.Time) string {
	timestamp := strconv.FormatInt(ts.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(body)
	return fmt.Sprintf("t=%s,v1=%s", timestamp, hex.EncodeToString(mac.Sum(nil)))
}

func (m *MockSuite) SignResend(body []byte) (http.Header, error) {
	wh, err := svix.NewWebhook(m.ResendSigningSecret)
	if err != nil {
		return nil, err
	}
	msgID := "msg_phase20"
	now := time.Now().UTC()
	timestamp := strconv.FormatInt(now.Unix(), 10)
	signature, err := wh.Sign(msgID, now, body)
	if err != nil {
		return nil, err
	}
	header := make(http.Header)
	header.Set("Content-Type", "application/json")
	header.Set("svix-id", msgID)
	header.Set("svix-timestamp", timestamp)
	header.Set("svix-signature", signature)
	return header, nil
}

func (m *MockSuite) SignedJWT(claims jwt.MapClaims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = m.jwksKeyID
	return token.SignedString(m.jwksPrivateKey)
}

func (m *MockSuite) handleSlack(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	_ = r.Body.Close()
	var payload map[string]any
	_ = json.Unmarshal(body, &payload)

	m.slackMu.Lock()
	m.slackMessages = append(m.slackMessages, payload)
	m.slackMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true,"ts":"1710000000.000100"}`))
}

func (m *MockSuite) handleNotion(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	_ = r.Body.Close()
	req := mockRequest{Method: r.Method, Path: r.URL.Path, Header: r.Header.Clone(), Body: body}
	m.notionMu.Lock()
	m.notionRequests = append(m.notionRequests, req)
	m.notionMu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/pages"):
		_, _ = w.Write([]byte(`{"id":"page_123"}`))
	case r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/blocks/"):
		_, _ = w.Write([]byte(`{"results":[{"id":"block_123"}]}`))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (m *MockSuite) handleResendAPI(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	_ = r.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/webhooks"):
		_, _ = w.Write([]byte(fmt.Sprintf(`{"id":"wh_123","signing_secret":"%s"}`, m.ResendSigningSecret)))
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/emails"):
		var payload map[string]any
		_ = json.Unmarshal(body, &payload)
		m.resendMu.Lock()
		m.resendEmails = append(m.resendEmails, payload)
		m.resendMu.Unlock()
		_, _ = w.Write([]byte(`{"id":"re_123"}`))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (m *MockSuite) handleLLM(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	_ = r.Body.Close()
	var payload struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	_ = json.Unmarshal(body, &payload)
	prompt := ""
	if len(payload.Messages) > 0 {
		prompt = payload.Messages[0].Content
	}
	m.llmMu.Lock()
	m.llmPrompts = append(m.llmPrompts, prompt)
	response := `"ok"`
	if len(m.llmQueue) > 0 {
		response = m.llmQueue[0]
		m.llmQueue = m.llmQueue[1:]
	}
	m.llmMu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(fmt.Sprintf(`{"choices":[{"message":{"content":%q}}],"usage":{"prompt_tokens":5,"completion_tokens":7,"total_tokens":12}}`, response)))
}

func (m *MockSuite) handleFunction(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	_ = r.Body.Close()
	req := mockRequest{Method: r.Method, Path: r.URL.Path, Header: r.Header.Clone(), Body: body}
	m.functionMu.Lock()
	m.functionCalls = append(m.functionCalls, req)
	m.functionMu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (m *MockSuite) handleJWKS(w http.ResponseWriter, _ *http.Request) {
	pub := m.jwksPrivateKey.PublicKey
	n := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(bigEndianBytes(pub.E))
	body := map[string]any{
		"keys": []map[string]any{{
			"kty": "RSA",
			"alg": "RS256",
			"use": "sig",
			"kid": m.jwksKeyID,
			"n":   n,
			"e":   e,
		}},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

func (m *MockSuite) initKeys() {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	m.jwksPrivateKey = key
}

func bigEndianBytes(v int) []byte {
	switch {
	case v == 0:
		return []byte{0}
	case v < 256:
		return []byte{byte(v)}
	default:
		var out []byte
		for x := v; x > 0; x >>= 8 {
			out = append([]byte{byte(x & 0xff)}, out...)
		}
		return out
	}
}

func cloneMapSlice(in []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(in))
	for _, item := range in {
		clone := make(map[string]any, len(item))
		for key, value := range item {
			clone[key] = value
		}
		out = append(out, clone)
	}
	return out
}

func PublicKeyPEM(privateKey *rsa.PrivateKey) string {
	der, _ := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

func SHA256Hex(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func RSASignPKCS1v15(privateKey *rsa.PrivateKey, digest []byte) (string, error) {
	sig, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}
