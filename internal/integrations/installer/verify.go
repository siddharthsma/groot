package installer

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"

	"groot/internal/integrations"
)

func loadTrustedKeys(path string) (TrustedKeysFile, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return TrustedKeysFile{}, fmt.Errorf("read trusted keys: %w", err)
	}
	var trusted TrustedKeysFile
	if err := jsonUnmarshal(body, &trusted); err != nil {
		return TrustedKeysFile{}, fmt.Errorf("decode trusted keys: %w", err)
	}
	return trusted, nil
}

func verifyPublisher(contents PackageContents, trusted TrustedKeysFile) (string, error) {
	signedHash := packageContentHash(contents.PluginBytes, contents.ManifestBytes)
	for _, publisher := range trusted.TrustedPublishers {
		key, err := decodePublicKey(publisher.PublicKey)
		if err != nil {
			return "", fmt.Errorf("decode trusted key for %s: %w", publisher.Name, err)
		}
		if ed25519.Verify(key, signedHash[:], contents.SignatureBytes) {
			return publisher.Name, nil
		}
	}
	return "", errors.New("package signature does not match any trusted publisher key")
}

func verifyManifest(manifest Manifest, currentVersion string) error {
	if strings.TrimSpace(manifest.Name) == "" {
		return errors.New("manifest.name is required")
	}
	if strings.TrimSpace(manifest.Version) == "" {
		return errors.New("manifest.version is required")
	}
	if strings.TrimSpace(manifest.IntegrationSpecHash) == "" {
		return errors.New("manifest.integration_spec_hash is required")
	}
	if strings.TrimSpace(manifest.BuildOS) == "" || strings.TrimSpace(manifest.BuildArch) == "" {
		return errors.New("manifest build_os and build_arch are required")
	}
	if manifest.BuildOS != runtime.GOOS || manifest.BuildArch != runtime.GOARCH {
		return fmt.Errorf("integration package targets %s/%s, current platform is %s/%s", manifest.BuildOS, manifest.BuildArch, runtime.GOOS, runtime.GOARCH)
	}
	if err := validateVersionConstraint(manifest.GrootVersion, currentVersion); err != nil {
		return err
	}
	return nil
}

func verifyIntegrationSpecHash(manifest Manifest, spec integration.IntegrationSpec) error {
	hash, err := integration.SpecHash(spec)
	if err != nil {
		return fmt.Errorf("hash integration spec: %w", err)
	}
	if hash != strings.TrimSpace(manifest.IntegrationSpecHash) {
		return fmt.Errorf("integration_spec_hash mismatch")
	}
	return nil
}

func verifyPackageChecksum(body []byte, checksum string) error {
	checksum = strings.TrimSpace(checksum)
	if checksum == "" {
		return nil
	}
	sum := sha256.Sum256(body)
	actual := fmt.Sprintf("sha256:%x", sum[:])
	if actual != checksum {
		return fmt.Errorf("package checksum mismatch")
	}
	return nil
}

func packageContentHash(pluginBytes, manifestBytes []byte) [32]byte {
	combined := make([]byte, 0, len(pluginBytes)+len(manifestBytes))
	combined = append(combined, pluginBytes...)
	combined = append(combined, manifestBytes...)
	return sha256.Sum256(combined)
}

func decodePublicKey(encoded string) (ed25519.PublicKey, error) {
	trimmed := strings.TrimSpace(encoded)
	trimmed = strings.TrimPrefix(trimmed, "ed25519:")
	decoded, err := base64.StdEncoding.DecodeString(trimmed)
	if err != nil {
		return nil, fmt.Errorf("decode base64: %w", err)
	}
	if len(decoded) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid ed25519 public key length")
	}
	return ed25519.PublicKey(decoded), nil
}

// jsonUnmarshal is isolated so tests can reuse signature/verification helpers
// without pulling in extra package-level state.
func jsonUnmarshal(body []byte, target any) error {
	return json.Unmarshal(body, target)
}
