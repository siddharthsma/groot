package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

const (
	ExecutionKindConnection = "connection"
	ExecutionKindFunction   = "function"
)

type Definition struct {
	Name            string
	ExecutionKind   string
	IntegrationName string
	Operation       string
	InputSchema     json.RawMessage

	schema *jsonschema.Schema
}

type Registry struct {
	definitions map[string]Definition
}

func DefaultDefinitions() []Definition {
	return []Definition{
		connectorTool("slack.post_message", "slack", "post_message", objectSchema(map[string]any{
			"channel": stringSchema(),
			"text":    stringSchema(),
			"blocks":  map[string]any{},
		}, []string{"text"})),
		connectorTool("slack.create_thread_reply", "slack", "create_thread_reply", objectSchema(map[string]any{
			"channel":   stringSchema(),
			"text":      stringSchema(),
			"thread_ts": stringSchema(),
		}, []string{"channel", "text", "thread_ts"})),
		connectorTool("notion.create_page", "notion", "create_page", objectSchema(map[string]any{
			"parent_database_id": stringSchema(),
			"properties":         map[string]any{"type": "object"},
		}, []string{"parent_database_id", "properties"})),
		connectorTool("notion.append_block", "notion", "append_block", objectSchema(map[string]any{
			"block_id": stringSchema(),
			"children": map[string]any{"type": "array", "minItems": 1},
		}, []string{"block_id", "children"})),
		connectorTool("resend.send_email", "resend", "send_email", objectSchema(map[string]any{
			"to":      stringSchema(),
			"subject": stringSchema(),
			"text":    stringSchema(),
			"html":    stringSchema(),
		}, []string{"to", "subject"})),
		functionTool("function.invoke"),
	}
}

func NewDefaultRegistry() (*Registry, error) {
	defs := DefaultDefinitions()
	definitions := make(map[string]Definition, len(defs))
	for _, def := range defs {
		compiled, err := compile(def.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("compile tool schema %s: %w", def.Name, err)
		}
		def.schema = compiled
		definitions[def.Name] = def
	}
	return &Registry{definitions: definitions}, nil
}

func (r *Registry) Get(name string) (Definition, bool) {
	def, ok := r.definitions[strings.TrimSpace(name)]
	return def, ok
}

func (r *Registry) Validate(name string, args json.RawMessage) error {
	def, ok := r.Get(name)
	if !ok {
		return fmt.Errorf("unknown tool: %s", name)
	}
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	var value any
	if err := json.Unmarshal(args, &value); err != nil {
		return fmt.Errorf("decode tool args: %w", err)
	}
	if err := def.schema.Validate(value); err != nil {
		return fmt.Errorf("validate tool args: %w", err)
	}
	return nil
}

func connectorTool(name, connectorName, operation string, schema map[string]any) Definition {
	body, _ := json.Marshal(schema)
	return Definition{
		Name:            name,
		ExecutionKind:   ExecutionKindConnection,
		IntegrationName: connectorName,
		Operation:       operation,
		InputSchema:     body,
	}
}

func functionTool(name string) Definition {
	body, _ := json.Marshal(map[string]any{
		"type": "object",
	})
	return Definition{
		Name:            name,
		ExecutionKind:   ExecutionKindFunction,
		IntegrationName: "function",
		Operation:       "invoke",
		InputSchema:     body,
	}
}

func compile(schema json.RawMessage) (*jsonschema.Schema, error) {
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("tool.json", strings.NewReader(string(schema))); err != nil {
		return nil, err
	}
	return compiler.Compile("tool.json")
}

func objectSchema(properties map[string]any, required []string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           properties,
		"required":             required,
	}
}

func stringSchema() map[string]any {
	return map[string]any{"type": "string"}
}
