package registry

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	"groot/internal/integrations"
	"groot/internal/schema"
)

type Source string

const (
	SourceCore   Source = "core"
	SourcePlugin Source = "plugin"
)

type Entry struct {
	Integration integration.Integration
	Source      Source
	Path        string
	Version     string
	Publisher   string
}

var (
	mu           sync.RWMutex
	integrations = map[string]Entry{}
)

func RegisterIntegration(p integration.Integration) {
	if err := register(p, SourceCore, "", true, "", ""); err != nil {
		panic(err.Error())
	}
}

func RegisterPlugin(path string, p integration.Integration) error {
	return RegisterPluginWithMetadata(path, p, "", "")
}

func RegisterPluginWithMetadata(path string, p integration.Integration, version string, publisher string) error {
	return register(p, SourcePlugin, strings.TrimSpace(path), false, strings.TrimSpace(version), strings.TrimSpace(publisher))
}

func register(p integration.Integration, source Source, path string, panicOnConflict bool, version string, publisher string) error {
	if p == nil {
		if panicOnConflict {
			panic("register integration: nil integration")
		}
		return fmt.Errorf("register integration: nil integration")
	}
	spec := p.Spec()
	if err := integration.ValidateSpec(spec); err != nil {
		if panicOnConflict {
			panic(fmt.Sprintf("register integration %q: %v", spec.Name, err))
		}
		return fmt.Errorf("register integration %q: %v", spec.Name, err)
	}
	name := strings.TrimSpace(spec.Name)

	mu.Lock()
	defer mu.Unlock()
	if _, exists := integrations[name]; exists {
		if panicOnConflict {
			panic(fmt.Sprintf("register integration %q: duplicate integration", name))
		}
		return fmt.Errorf("register integration %q: duplicate integration", name)
	}
	integrations[name] = Entry{Integration: p, Source: source, Path: path, Version: version, Publisher: publisher}
	return nil
}

func GetIntegration(name string) integration.Integration {
	entry, ok := GetEntry(name)
	if !ok {
		return nil
	}
	return entry.Integration
}

func GetEntry(name string) (Entry, bool) {
	mu.RLock()
	defer mu.RUnlock()
	entry, ok := integrations[strings.TrimSpace(name)]
	return entry, ok
}

func ListIntegrations() []integration.Integration {
	entries := ListEntries()
	out := make([]integration.Integration, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.Integration)
	}
	return out
}

func ListEntries() []Entry {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Entry, 0, len(integrations))
	for _, registered := range integrations {
		out = append(out, registered)
	}
	slices.SortFunc(out, func(a, b Entry) int {
		return strings.Compare(a.Integration.Spec().Name, b.Integration.Spec().Name)
	})
	return out
}

func FindIntegrationByOperation(operation string) (string, bool) {
	target := strings.TrimSpace(operation)
	if target == "" {
		return "", false
	}
	var match string
	for _, entry := range ListEntries() {
		for _, op := range entry.Integration.Spec().Operations {
			if strings.TrimSpace(op.Name) != target {
				continue
			}
			if match != "" && match != entry.Integration.Spec().Name {
				return "", false
			}
			match = entry.Integration.Spec().Name
		}
	}
	return match, match != ""
}

func Bundles() []schema.Bundle {
	return BundlesBySource()
}

func BundlesBySource(sources ...Source) []schema.Bundle {
	list := ListEntries()
	allowed := make(map[Source]struct{}, len(sources))
	for _, source := range sources {
		allowed[source] = struct{}{}
	}
	bundles := make([]schema.Bundle, 0, len(list))
	for _, entry := range list {
		if len(allowed) > 0 {
			if _, ok := allowed[entry.Source]; !ok {
				continue
			}
		}
		spec := entry.Integration.Spec()
		bundle := schema.Bundle{Name: spec.Name}
		for _, declared := range spec.Schemas {
			bundle.Schemas = append(bundle.Schemas, schema.Spec{
				EventType:  strings.TrimSpace(declared.EventType),
				Version:    declared.Version,
				Source:     spec.Name,
				SourceKind: strings.TrimSpace(declared.SourceKind),
				SchemaJSON: declared.SchemaJSON,
			})
		}
		bundles = append(bundles, bundle)
	}
	return bundles
}

func Validate() error {
	for _, registered := range ListEntries() {
		if err := integration.ValidateSpec(registered.Integration.Spec()); err != nil {
			return err
		}
	}
	return nil
}
