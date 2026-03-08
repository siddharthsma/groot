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

	"groot/internal/connectors/installer"
	"groot/internal/connectors/pluginloader"
	"groot/internal/connectors/provider"
	"groot/tests/helpers"
)

var (
	exampleSpecHashOnce sync.Once
	exampleSpecHashValue string
	exampleSpecHashErr error
)

func TestPhase30ProviderInstallFromRegistryAndCatalog(t *testing.T) {
	if !pluginSupported() {
		t.Skipf("go plugins are not supported on %s", runtime.GOOS)
	}
	pluginBuildDir := t.TempDir()
	buildExamplePlugin(t, pluginBuildDir)
	pluginPath := filepath.Join(pluginBuildDir, "example_echo_provider.so")

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}

	paths := phase30Paths(t)
	writeTrustedKeys(t, paths.trustedKeysPath, "Test Publisher", pub)
	pkgBytes := buildProviderPackage(t, pluginPath, priv, installer.Manifest{
		Name:         "example_echo_provider",
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
				Providers: []installer.RegistryProvider{{
					Name: "example_echo_provider",
					Versions: []installer.RegistryVersion{{
						Version:    "1.0.0",
						PackageURL: packageURL,
						Checksum:   "sha256:" + strings.ToLower(base16(sum[:])),
					}},
				}},
			})
		case "/example_echo_provider-1.0.0.grootpkg":
			_, _ = w.Write(pkgBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	packageURL = server.URL + "/example_echo_provider-1.0.0.grootpkg"

	cli := buildGrootCLI(t, "1.2.3")
	output := runCLI(t, cli, phase30Env(paths, server.URL+"/index.json"), "provider", "install", "example_echo_provider")
	if !strings.Contains(output, `"name": "example_echo_provider"`) {
		t.Fatalf("install output = %s", output)
	}

	if _, err := os.Stat(filepath.Join(paths.pluginDir, "example_echo_provider.so")); err != nil {
		t.Fatalf("installed plugin missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(paths.cacheDir, "example_echo_provider-1.0.0.grootpkg")); err != nil {
		t.Fatalf("cached package missing: %v", err)
	}

	h := helpers.NewHarness(t, helpers.HarnessOptions{
		ExtraEnv: map[string]string{
			"GROOT_PROVIDER_PLUGIN_DIR":        paths.pluginDir,
			"GROOT_PROVIDER_INSTALLED_PATH":    paths.installedPath,
			"GROOT_PROVIDER_TRUSTED_KEYS_PATH": paths.trustedKeysPath,
			"GROOT_PROVIDER_CACHE_DIR":         paths.cacheDir,
			"GROOT_PROVIDER_REGISTRY_URL":      server.URL + "/index.json",
		},
	})

	resp, body := h.Request(http.MethodGet, "/providers/example_echo_provider", nil, nil)
	mustStatus(t, resp, body, http.StatusOK)
	if !strings.Contains(string(body), `"source":"plugin"`) || !strings.Contains(string(body), `"version":"1.0.0"`) || !strings.Contains(string(body), `"publisher":"Test Publisher"`) {
		t.Fatalf("provider body = %s", string(body))
	}

	listOutput := runCLI(t, cli, phase30Env(paths, server.URL+"/index.json"), "provider", "list")
	if !strings.Contains(listOutput, `"version": "1.0.0"`) {
		t.Fatalf("list output = %s", listOutput)
	}
	infoOutput := runCLI(t, cli, phase30Env(paths, server.URL+"/index.json"), "provider", "info", "example_echo_provider")
	if !strings.Contains(infoOutput, `"publisher": "Test Publisher"`) {
		t.Fatalf("info output = %s", infoOutput)
	}

	runCLI(t, cli, phase30Env(paths, server.URL+"/index.json"), "provider", "remove", "example_echo_provider")
	if _, err := os.Stat(filepath.Join(paths.pluginDir, "example_echo_provider.so")); !os.IsNotExist(err) {
		t.Fatalf("plugin still present after remove: %v", err)
	}
	afterRemove := runCLI(t, cli, phase30Env(paths, server.URL+"/index.json"), "provider", "list")
	if strings.Contains(afterRemove, "example_echo_provider") {
		t.Fatalf("list after remove = %s", afterRemove)
	}
}

func TestPhase30RejectsInvalidSignature(t *testing.T) {
	if !pluginSupported() {
		t.Skipf("go plugins are not supported on %s", runtime.GOOS)
	}
	pluginBuildDir := t.TempDir()
	buildExamplePlugin(t, pluginBuildDir)
	pluginPath := filepath.Join(pluginBuildDir, "example_echo_provider.so")
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
	pkgBytes := buildProviderPackage(t, pluginPath, wrongPriv, installer.Manifest{
		Name:         "example_echo_provider",
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
	errOutput := runCLIExpectFailure(t, cli, phase30Env(paths, ""), "provider", "install", pkgPath)
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
	pluginPath := filepath.Join(pluginBuildDir, "example_echo_provider.so")
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	paths := phase30Paths(t)
	writeTrustedKeys(t, paths.trustedKeysPath, "Test Publisher", pub)
	pkgBytes := buildProviderPackage(t, pluginPath, priv, installer.Manifest{
		Name:         "example_echo_provider",
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
	errOutput := runCLIExpectFailure(t, cli, phase30Env(paths, ""), "provider", "install", pkgPath)
	if !strings.Contains(errOutput, "groot_version") {
		t.Fatalf("stderr = %s", errOutput)
	}
}

type providerPaths struct {
	pluginDir        string
	trustedKeysPath  string
	installedPath    string
	cacheDir         string
}

func phase30Paths(t *testing.T) providerPaths {
	t.Helper()
	base := t.TempDir()
	return providerPaths{
		pluginDir:       filepath.Join(base, "plugins"),
		trustedKeysPath: filepath.Join(base, "trusted_keys.json"),
		installedPath:   filepath.Join(base, "installed.json"),
		cacheDir:        filepath.Join(base, "cache"),
	}
}

func phase30Env(paths providerPaths, registryURL string) []string {
	env := append(os.Environ(),
		"GROOT_PROVIDER_PLUGIN_DIR="+paths.pluginDir,
		"GROOT_PROVIDER_TRUSTED_KEYS_PATH="+paths.trustedKeysPath,
		"GROOT_PROVIDER_INSTALLED_PATH="+paths.installedPath,
		"GROOT_PROVIDER_CACHE_DIR="+paths.cacheDir,
	)
	if strings.TrimSpace(registryURL) != "" {
		env = append(env, "GROOT_PROVIDER_REGISTRY_URL="+registryURL)
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

func buildProviderPackage(t *testing.T, pluginPath string, privateKey ed25519.PrivateKey, manifest installer.Manifest) []byte {
	t.Helper()
	hash := exampleProviderSpecHash(t, pluginPath)
	manifest.ProviderSpecHash = hash
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
	writeTarFile(t, tw, "provider/provider.so", pluginBytes, 0o755)
	writeTarFile(t, tw, "provider/manifest.json", manifestBytes, 0o644)
	writeTarFile(t, tw, "provider/signature.ed25519", signature, 0o644)
	if err := tw.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	return buf.Bytes()
}

func exampleProviderSpecHash(t *testing.T, pluginPath string) string {
	t.Helper()
	exampleSpecHashOnce.Do(func() {
		loaded, err := pluginloader.Open(pluginPath)
		if err != nil {
			exampleSpecHashErr = err
			return
		}
		exampleSpecHashValue, exampleSpecHashErr = provider.SpecHash(loaded.Spec())
	})
	if exampleSpecHashErr != nil {
		t.Fatalf("exampleProviderSpecHash() error = %v", exampleSpecHashErr)
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
