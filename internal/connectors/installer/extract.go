package installer

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func extractPackage(body []byte) (PackageContents, error) {
	reader := tar.NewReader(bytes.NewReader(body))
	var contents PackageContents
	foundPlugin := false
	foundManifest := false
	foundSignature := false
	for {
		header, err := reader.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return PackageContents{}, fmt.Errorf("read package tar: %w", err)
		}
		switch normalizePackagePath(header.Name) {
		case "provider/provider.so":
			contents.PluginBytes, err = io.ReadAll(reader)
			if err != nil {
				return PackageContents{}, fmt.Errorf("read provider.so: %w", err)
			}
			foundPlugin = true
		case "provider/manifest.json":
			contents.ManifestBytes, err = io.ReadAll(reader)
			if err != nil {
				return PackageContents{}, fmt.Errorf("read manifest.json: %w", err)
			}
			if err := json.Unmarshal(contents.ManifestBytes, &contents.Manifest); err != nil {
				return PackageContents{}, fmt.Errorf("decode manifest.json: %w", err)
			}
			foundManifest = true
		case "provider/signature.ed25519":
			contents.SignatureBytes, err = io.ReadAll(reader)
			if err != nil {
				return PackageContents{}, fmt.Errorf("read signature.ed25519: %w", err)
			}
			foundSignature = true
		}
	}
	if !foundPlugin || !foundManifest || !foundSignature {
		return PackageContents{}, fmt.Errorf("package must contain provider/provider.so, provider/manifest.json, and provider/signature.ed25519")
	}
	return contents, nil
}

func normalizePackagePath(path string) string {
	return strings.Trim(strings.ReplaceAll(path, "\\", "/"), "/")
}
