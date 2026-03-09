//go:build integration

package integration

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"groot/internal/integrations/installer"
	"groot/internal/integrations/pluginloader"
	"groot/internal/integrations"
	"groot/tests/helpers"
)

var (
	exampleSpecHashOnce sync.Once
	exampleSpecHashValue string
	exampleSpecHashErr error
)

func TestPhase30IntegrationInstallFromRegistryAndCatalog(t *testing.T) {
	if !pluginSupported() {
		t.Skipf("go plugins are not supported on %s", runtime.GOOS)
	}
	pluginBuildDir := t.TempDir()
	buildExamplePlugin(t, pluginBuildDir)
	pluginPath := filepath.Join(pluginBuildDir, "example_echo_integration.so")

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}

	paths := phase30Paths(t)
	writeTrustedKeys(t, paths.trustedKeysPath, "Test Publisher", pub)
	pkgBytes := buildIntegrationPackage(t, pluginPath, priv, installer.Manifest{
		Name:         "example_echo_integration",
		Version:      "1.0.0",
		Description:  "Echo plugin",
		Author:       "Phase30 Test",
		GrootVersion: ">=1.0.0",
		BuildOS:      runtime.GOOS,
		BuildArch:    runtime.GOARCH,
	})

	var packageURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.json":
			sum := sha256.Sum256(pkgBytes)
			_ = json.NewEncoder(w).Encode(installer.RegistryIndex{
				Integrations: []installer.RegistryIntegration{{
					Name: "example_echo_integration",
					Versions: []installer.RegistryVersion{{
						Version:    "1.0.0",
						PackageURL: packageURL,
						Checksum:   "sha256:" + strings.ToLower(base16(sum[:])),
					}},
				}},
			})
		case "/example_echo_integration-1.0.0.grootpkg":
			_, _ = w.Write(pkgBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	packageURL = server.URL + "/example_echo_integration-1.0.0.grootpkg"

	cli := buildGrootCLI(t, "1.2.3")
	output := runCLI(t, cli, phase30Env(paths, server.URL+"/index.json"), "integration", "install", "example_echo_integration")
	if !strings.Contains(output, `"name": "example_echo_integration"`) {
		t.Fatalf("install output = %s", output)
	}

	if _, err := os.Stat(filepath.Join(paths.pluginDir, "example_echo_integration.so")); err != nil {
		t.Fatalf("installed plugin missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(paths.cacheDir, "example_echo_integration-1.0.0.grootpkg")); err != nil {
		t.Fatalf("cached package missing: %v", err)
	}

	h := helpers.NewHarness(t, helpers.HarnessOptions{
		ExtraEnv: map[string]string{
			"GROOT_INTEGRATION_PLUGIN_DIR":        paths.pluginDir,
			"GROOT_INTEGRATION_INSTALLED_PATH":    paths.installedPath,
			"GROOT_INTEGRATION_TRUSTED_KEYS_PATH": paths.trustedKeysPath,
			"GROOT_INTEGRATION_CACHE_DIR":         paths.cacheDir,
			"GROOT_INTEGRATION_REGISTRY_URL":      server.URL + "/index.json",
		},
	})

	resp, body := h.Request(http.MethodGet, "/integrations/example_echo_integration", nil, nil)
	mustStatus(t, resp, body, http.StatusOK)
	if !strings.Contains(string(body), `"source":"plugin"`) || !strings.Contains(string(body), `"version":"1.0.0"`) || !strings.Contains(string(body), `"publisher":"Test Publisher"`) {
		t.Fatalf("integration body = %s", string(body))
	}

	listOutput := runCLI(t, cli, phase30Env(paths, server.URL+"/index.json"), "integration", "list")
	if !strings.Contains(listOutput, `"version": "1.0.0"`) {
		t.Fatalf("list output = %s", listOutput)
	}
	infoOutput := runCLI(t, cli, phase30Env(paths, server.URL+"/index.json"), "integration", "info", "example_echo_integration")
	if !strings.Contains(infoOutput, `"publisher": "Test Publisher"`) {
		t.Fatalf("info output = %s", infoOutput)
	}

	runCLI(t, cli, phase30Env(paths, server.URL+"/index.json"), "integration", "remove", "example_echo_integration")
	if _, err := os.Stat(filepath.Join(paths.pluginDir, "example_echo_integration.so")); !os.IsNotExist(err) {
		t.Fatalf("plugin still present after remove: %v", err)
	}
	afterRemove := runCLI(t, cli, phase30Env(paths, server.URL+"/index.json"), "integration", "list")
	if strings.Contains(afterRemove, "example_echo_integration") {
		t.Fatalf("list after remove = %s", afterRemove)
	}
}

func TestPhase30RejectsInvalidSignature(t *testing.T) {
	if !pluginSupported() {
		t.Skipf("go plugins are not supported on %s", runtime.GOOS)
	}
	pluginBuildDir := t.TempDir()
	buildExamplePlugin(t, pluginBuildDir)
	pluginPath := filepath.Join(pluginBuildDir, "example_echo_integration.so")
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	_, wrongPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	paths := phase30Paths(t)
	writeTrustedKeys(t, paths.trustedKeysPath, "Test Publisher", pub)
	pkgBytes := buildIntegrationPackage(t, pluginPath, wrongPriv, installer.Manifest{
		Name:         "example_echo_integration",
		Version:      "1.0.0",
		Description:  "Echo plugin",
		Author:       "Phase30 Test",
		GrootVersion: ">=1.0.0",
		BuildOS:      runtime.GOOS,
		BuildArch:    runtime.GOARCH,
	})
	pkgPath := filepath.Join(t.TempDir(), "bad.grootpkg")
	if err := os.WriteFile(pkgPath, pkgBytes, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cli := buildGrootCLI(t, "1.2.3")
	errOutput := runCLIExpectFailure(t, cli, phase30Env(paths, ""), "integration", "install", pkgPath)
	if !strings.Contains(errOutput, "signature") {
		t.Fatalf("stderr = %s", errOutput)
	}
}

func TestPhase30RejectsIncompatibleVersion(t *testing.T) {
	if !pluginSupported() {
		t.Skipf("go plugins are not supported on %s", runtime.GOOS)
	}
	pluginBuildDir := t.TempDir()
	buildExamplePlugin(t, pluginBuildDir)
	pluginPath := filepath.Join(pluginBuildDir, "example_echo_integration.so")
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	paths := phase30Paths(t)
	writeTrustedKeys(t, paths.trustedKeysPath, "Test Publisher", pub)
	pkgBytes := buildIntegrationPackage(t, pluginPath, priv, installer.Manifest{
		Name:         "example_echo_integration",
		Version:      "1.0.0",
		Description:  "Echo plugin",
		Author:       "Phase30 Test",
		GrootVersion: ">=9.0.0",
		BuildOS:      runtime.GOOS,
		BuildArch:    runtime.GOARCH,
	})
	pkgPath := filepath.Join(t.TempDir(), "bad-version.grootpkg")
	if err := os.WriteFile(pkgPath, pkgBytes, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cli := buildGrootCLI(t, "1.2.3")
	errOutput := runCLIExpectFailure(t, cli, phase30Env(paths, ""), "integration", "install", pkgPath)
	if !strings.Contains(errOutput, "groot_version") {
		t.Fatalf("stderr = %s", errOutput)
	}
}

type integrationPaths struct {
	pluginDir        string
	trustedKeysPath  string
	installedPath    string
	cacheDir         string
}

func phase30Paths(t *testing.T) integrationPaths {
	t.Helper()
	base := t.TempDir()
	return integrationPaths{
		pluginDir:       filepath.Join(base, "plugins"),
		trustedKeysPath: filepath.Join(base, "trusted_keys.json"),
		installedPath:   filepath.Join(base, "installed.json"),
		cacheDir:        filepath.Join(base, "cache"),
	}
}

func phase30Env(paths integrationPaths, registryURL string) []string {
	env := append(os.Environ(),
		"GROOT_INTEGRATION_PLUGIN_DIR="+paths.pluginDir,
		"GROOT_INTEGRATION_TRUSTED_KEYS_PATH="+paths.trustedKeysPath,
		"GROOT_INTEGRATION_INSTALLED_PATH="+paths.installedPath,
		"GROOT_INTEGRATION_CACHE_DIR="+paths.cacheDir,
	)
	if strings.TrimSpace(registryURL) != "" {
		env = append(env, "GROOT_INTEGRATION_REGISTRY_URL="+registryURL)
	}
	return env
}

func writeTrustedKeys(t *testing.T, path string, name string, publicKey ed25519.PublicKey) {
	t.Helper()
	body, err := json.MarshalIndent(installer.TrustedKeysFile{
		TrustedPublishers: []installer.TrustedPublisher{{
			Name:      name,
			PublicKey: "ed25519:" + base64.StdEncoding.EncodeToString(publicKey),
		}},
	}, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func buildIntegrationPackage(t *testing.T, pluginPath string, privateKey ed25519.PrivateKey, manifest installer.Manifest) []byte {
	t.Helper()
	hash := exampleIntegrationSpecHash(t, pluginPath)
	manifest.IntegrationSpecHash = hash
	pluginBytes, err := os.ReadFile(pluginPath)
	if err != nil {
		t.Fatalf("ReadFile(plugin) error = %v", err)
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("Marshal(manifest) error = %v", err)
	}
	digest := sha256.Sum256(append(append([]byte(nil), pluginBytes...), manifestBytes...))
	signature := ed25519.Sign(privateKey, digest[:])
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	writeTarFile(t, tw, "integration/integration.so", pluginBytes, 0o755)
	writeTarFile(t, tw, "integration/manifest.json", manifestBytes, 0o644)
	writeTarFile(t, tw, "integration/signature.ed25519", signature, 0o644)
	if err := tw.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	return buf.Bytes()
}

func exampleIntegrationSpecHash(t *testing.T, pluginPath string) string {
	t.Helper()
	exampleSpecHashOnce.Do(func() {
		loaded, err := pluginloader.Open(pluginPath)
		if err != nil {
			exampleSpecHashErr = err
			return
		}
		exampleSpecHashValue, exampleSpecHashErr = integration.SpecHash(loaded.Spec())
	})
	if exampleSpecHashErr != nil {
		t.Fatalf("exampleIntegrationSpecHash() error = %v", exampleSpecHashErr)
	}
	return exampleSpecHashValue
}

func writeTarFile(t *testing.T, tw *tar.Writer, name string, body []byte, mode int64) {
	t.Helper()
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: int64(len(body))}); err != nil {
		t.Fatalf("WriteHeader() error = %v", err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
}

func buildGrootCLI(t *testing.T, version string) string {
	t.Helper()
	root := helpers.RepoRoot()
	binDir := t.TempDir()
	binary := filepath.Join(binDir, "groot")
	cmd := exec.Command("go", "build", "-ldflags", "-X main.BuildVersion="+version, "-o", binary, "./cmd/groot")
	cmd.Dir = root
	var logs bytes.Buffer
	cmd.Stdout = &logs
	cmd.Stderr = &logs
	if err := cmd.Run(); err != nil {
		t.Fatalf("build cli: %v\n%s", err, logs.String())
	}
	return binary
}

func runCLI(t *testing.T, binary string, env []string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), binary, args...)
	cmd.Env = env
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("run cli: %v\n%s", err, out.String())
	}
	return out.String()
}

func runCLIExpectFailure(t *testing.T, binary string, env []string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), binary, args...)
	cmd.Env = env
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err == nil {
		t.Fatalf("expected command to fail\n%s", out.String())
	}
	return out.String()
}

func base16(body []byte) string {
	const hex = "0123456789abcdef"
	out := make([]byte, len(body)*2)
	for i, b := range body {
		out[i*2] = hex[b>>4]
		out[i*2+1] = hex[b&0x0f]
	}
	return string(out)
}
