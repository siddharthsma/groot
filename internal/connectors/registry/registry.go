package registry

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	"groot/internal/connectors/provider"
	"groot/internal/schema"
)

type Source string

const (
	SourceCore   Source = "core"
	SourcePlugin Source = "plugin"
)

type Entry struct {
	Provider  provider.Provider
	Source    Source
	Path      string
	Version   string
	Publisher string
}

var (
	mu        sync.RWMutex
	providers = map[string]Entry{}
)

func RegisterProvider(p provider.Provider) {
	if err := register(p, SourceCore, "", true, "", ""); err != nil {
		panic(err.Error())
	}
}

func RegisterPlugin(path string, p provider.Provider) error {
	return RegisterPluginWithMetadata(path, p, "", "")
}

func RegisterPluginWithMetadata(path string, p provider.Provider, version string, publisher string) error {
	return register(p, SourcePlugin, strings.TrimSpace(path), false, strings.TrimSpace(version), strings.TrimSpace(publisher))
}

func register(p provider.Provider, source Source, path string, panicOnConflict bool, version string, publisher string) error {
	if p == nil {
		if panicOnConflict {
			panic("register provider: nil provider")
		}
		return fmt.Errorf("register provider: nil provider")
	}
	spec := p.Spec()
	if err := provider.ValidateSpec(spec); err != nil {
		if panicOnConflict {
			panic(fmt.Sprintf("register provider %q: %v", spec.Name, err))
		}
		return fmt.Errorf("register provider %q: %v", spec.Name, err)
	}
	name := strings.TrimSpace(spec.Name)

	mu.Lock()
	defer mu.Unlock()
	if _, exists := providers[name]; exists {
		if panicOnConflict {
			panic(fmt.Sprintf("register provider %q: duplicate provider", name))
		}
		return fmt.Errorf("register provider %q: duplicate provider", name)
	}
	providers[name] = Entry{Provider: p, Source: source, Path: path, Version: version, Publisher: publisher}
	return nil
}

func GetProvider(name string) provider.Provider {
	entry, ok := GetEntry(name)
	if !ok {
		return nil
	}
	return entry.Provider
}

func GetEntry(name string) (Entry, bool) {
	mu.RLock()
	defer mu.RUnlock()
	entry, ok := providers[strings.TrimSpace(name)]
	return entry, ok
}

func ListProviders() []provider.Provider {
	entries := ListEntries()
	out := make([]provider.Provider, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.Provider)
	}
	return out
}

func ListEntries() []Entry {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Entry, 0, len(providers))
	for _, registered := range providers {
		out = append(out, registered)
	}
	slices.SortFunc(out, func(a, b Entry) int {
		return strings.Compare(a.Provider.Spec().Name, b.Provider.Spec().Name)
	})
	return out
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
		spec := entry.Provider.Spec()
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
		if err := provider.ValidateSpec(registered.Provider.Spec()); err != nil {
			return err
		}
	}
	return nil
}
