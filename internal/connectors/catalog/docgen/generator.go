package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"groot/internal/connectors/catalog"
)

func main() {
	root, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	outDir := filepath.Join(root, "docs", "providers", "generated")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		panic(err)
	}
	svc := catalog.NewService(nil)
	providers, err := svc.List(context.Background())
	if err != nil {
		panic(err)
	}
	for _, summary := range providers {
		detail, err := svc.Get(context.Background(), summary.Name)
		if err != nil {
			panic(err)
		}
		if err := os.WriteFile(filepath.Join(outDir, summary.Name+".md"), []byte(render(detail)), 0o644); err != nil {
			panic(err)
		}
	}
}

func render(detail catalog.ProviderDetail) string {
	var b strings.Builder
	b.WriteString("# " + title(detail.Name) + " Provider\n\n")
	b.WriteString("## Provider Name\n\n")
	b.WriteString(detail.Name + "\n\n")
	b.WriteString("## Source\n\n")
	b.WriteString(detail.Source + "\n\n")
	b.WriteString("## Supported Scopes\n\n")
	switch {
	case detail.SupportsTenantScope && detail.SupportsGlobalScope:
		b.WriteString("- tenant\n- global\n\n")
	case detail.SupportsTenantScope:
		b.WriteString("- tenant\n\n")
	case detail.SupportsGlobalScope:
		b.WriteString("- global\n\n")
	default:
		b.WriteString("- none\n\n")
	}
	b.WriteString("## Inbound Events\n\n")
	if detail.Inbound == nil || len(detail.Inbound.EventTypes) == 0 {
		b.WriteString("None.\n\n")
	} else {
		for _, eventType := range detail.Inbound.EventTypes {
			b.WriteString("- " + eventType + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("## Operations\n\n")
	if len(detail.Operations) == 0 {
		b.WriteString("None.\n\n")
	} else {
		for _, op := range detail.Operations {
			b.WriteString(fmt.Sprintf("- `%s`: %s\n", op.Name, op.Description))
		}
		b.WriteString("\n")
	}
	b.WriteString("## Config Fields\n\n")
	if len(detail.Config.Fields) == 0 {
		b.WriteString("None.\n\n")
	} else {
		for _, field := range detail.Config.Fields {
			b.WriteString(fmt.Sprintf("- `%s` required=%t secret=%t\n", field.Name, field.Required, field.Secret))
		}
		b.WriteString("\n")
	}
	b.WriteString("## Schemas\n\n")
	if len(detail.Schemas) == 0 {
		b.WriteString("None.\n")
	} else {
		for _, schema := range detail.Schemas {
			b.WriteString(fmt.Sprintf("- `%s.v%d`\n", schema.EventType, schema.Version))
		}
	}
	return b.String()
}

func title(value string) string {
	runes := []rune(value)
	if len(runes) == 0 {
		return value
	}
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}
