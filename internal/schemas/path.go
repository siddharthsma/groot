package schemas

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

var placeholderPattern = regexp.MustCompile(`\{\{([^{}]+)\}\}`)

type schemaNode struct {
	Kind                 string
	Properties           map[string]schemaNode
	Items                *schemaNode
	AllowAdditional      bool
	AdditionalProperties *schemaNode
}

func parseSchema(raw json.RawMessage) (schemaNode, error) {
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return schemaNode{}, err
	}
	return buildSchemaNode(decoded), nil
}

func buildSchemaNode(decoded map[string]any) schemaNode {
	node := schemaNode{Properties: make(map[string]schemaNode)}
	switch typed := decoded["type"].(type) {
	case string:
		node.Kind = typed
	case []any:
		for _, item := range typed {
			if value, ok := item.(string); ok && value != "null" {
				node.Kind = value
				break
			}
		}
	}
	if properties, ok := decoded["properties"].(map[string]any); ok {
		for key, value := range properties {
			if child, ok := value.(map[string]any); ok {
				node.Properties[key] = buildSchemaNode(child)
			}
		}
	}
	if items, ok := decoded["items"].(map[string]any); ok {
		child := buildSchemaNode(items)
		node.Items = &child
	}
	switch typed := decoded["additionalProperties"].(type) {
	case bool:
		node.AllowAdditional = typed
	case map[string]any:
		child := buildSchemaNode(typed)
		node.AdditionalProperties = &child
	}
	return node
}

func (n schemaNode) allows(tokens []string) bool {
	if len(tokens) == 0 {
		return true
	}
	token := tokens[0]
	if token == "*" {
		if n.Items == nil {
			return false
		}
		return n.Items.allows(tokens[1:])
	}
	if child, ok := n.Properties[token]; ok {
		return child.allows(tokens[1:])
	}
	if n.AdditionalProperties != nil {
		return n.AdditionalProperties.allows(tokens[1:])
	}
	if n.AllowAdditional {
		return true
	}
	return false
}

func parsePayloadTokens(key string) ([]string, error) {
	trimmed := strings.TrimSpace(key)
	if trimmed == "payload" {
		return nil, nil
	}
	if !strings.HasPrefix(trimmed, "payload") {
		return nil, errors.New("template must start with payload")
	}
	rest := strings.TrimPrefix(trimmed, "payload")
	if rest == "" {
		return nil, nil
	}
	var tokens []string
	for len(rest) > 0 {
		switch {
		case strings.HasPrefix(rest, "."):
			rest = rest[1:]
			next := nextTokenEnd(rest)
			if next == 0 {
				return nil, errors.New("empty payload token")
			}
			tokens = append(tokens, rest[:next])
			rest = rest[next:]
		case strings.HasPrefix(rest, "["):
			end := strings.Index(rest, "]")
			if end <= 1 {
				return nil, errors.New("invalid payload index")
			}
			tokens = append(tokens, "*")
			rest = rest[end+1:]
		default:
			return nil, errors.New("invalid payload token")
		}
	}
	return tokens, nil
}

func nextTokenEnd(value string) int {
	for idx, r := range value {
		if r == '.' || r == '[' {
			return idx
		}
	}
	return len(value)
}
