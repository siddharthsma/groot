package subscriptionfilter

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"groot/internal/schema"
)

const WarningSchemaMissing = "schema_missing_for_event_type"

type SchemaLookup interface {
	Get(context.Context, string) (schema.Schema, error)
}

type Service struct {
	schemas SchemaLookup
}

type ValidationError struct {
	InvalidPaths []string `json:"invalid_paths"`
	InvalidOps   []string `json:"invalid_ops"`
}

func (e ValidationError) Error() string {
	return "subscription filter is invalid"
}

type Filter interface {
	isFilter()
}

type Group struct {
	All []Filter `json:"all,omitempty"`
	Any []Filter `json:"any,omitempty"`
	Not Filter   `json:"not,omitempty"`
}

type Condition struct {
	Path  string `json:"path"`
	Op    string `json:"op"`
	Value any    `json:"value,omitempty"`
}

func (Group) isFilter()     {}
func (Condition) isFilter() {}

func NewService(schemas SchemaLookup) *Service {
	return &Service{schemas: schemas}
}

func (s *Service) Validate(ctx context.Context, eventType string, raw json.RawMessage) (json.RawMessage, []string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil, nil
	}
	filter, err := Parse(raw)
	if err != nil {
		return nil, nil, err
	}
	if err := validateStructure(filter, 1, &counter{}); err != nil {
		return nil, nil, err
	}
	normalized, err := Marshal(filter)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal filter: %w", err)
	}
	if s == nil || s.schemas == nil {
		return normalized, nil, nil
	}
	schema, err := s.schemas.Get(ctx, strings.TrimSpace(eventType))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return normalized, []string{WarningSchemaMissing}, nil
		}
		return nil, nil, fmt.Errorf("get schema: %w", err)
	}
	root, err := parseSchemaNode(schema.SchemaJSON)
	if err != nil {
		return nil, nil, fmt.Errorf("parse schema: %w", err)
	}
	validationErr := ValidationError{}
	validateAgainstSchema(filter, root, &validationErr)
	if len(validationErr.InvalidPaths) > 0 || len(validationErr.InvalidOps) > 0 {
		return nil, nil, validationErr
	}
	return normalized, nil, nil
}

func Parse(raw json.RawMessage) (Filter, error) {
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, ValidationError{}
	}
	filter, err := parseFilterValue(decoded)
	if err != nil {
		return nil, err
	}
	return filter, nil
}

func Marshal(filter Filter) (json.RawMessage, error) {
	body, err := json.Marshal(filter)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(body), nil
}

func Evaluate(raw json.RawMessage, payload json.RawMessage) (bool, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return true, nil
	}
	filter, err := Parse(raw)
	if err != nil {
		return false, err
	}
	var decoded any
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &decoded); err != nil {
			return false, fmt.Errorf("decode payload: %w", err)
		}
	}
	root, _ := decoded.(map[string]any)
	return evalFilter(filter, root), nil
}

type counter struct {
	conditions int
}

func validateStructure(filter Filter, depth int, counts *counter) error {
	if depth > 10 {
		return ValidationError{}
	}
	switch typed := filter.(type) {
	case Group:
		switch {
		case len(typed.All) > 0:
			if len(typed.All) == 0 {
				return ValidationError{}
			}
			for _, child := range typed.All {
				if err := validateStructure(child, depth+1, counts); err != nil {
					return err
				}
			}
		case len(typed.Any) > 0:
			if len(typed.Any) == 0 {
				return ValidationError{}
			}
			for _, child := range typed.Any {
				if err := validateStructure(child, depth+1, counts); err != nil {
					return err
				}
			}
		case typed.Not != nil:
			return validateStructure(typed.Not, depth+1, counts)
		default:
			return ValidationError{}
		}
	case Condition:
		counts.conditions++
		if counts.conditions > 50 {
			return ValidationError{}
		}
		if err := validatePath(typed.Path); err != nil {
			return ValidationError{InvalidPaths: []string{typed.Path}}
		}
		if strings.TrimSpace(typed.Op) == "" {
			return ValidationError{InvalidOps: []string{typed.Path + ":missing_op"}}
		}
	}
	return nil
}

func validatePath(path string) error {
	trimmed := strings.TrimSpace(path)
	if !strings.HasPrefix(trimmed, "payload.") {
		return errors.New("path must start with payload.")
	}
	if strings.Contains(trimmed, "[") || strings.Contains(trimmed, "]") || strings.Contains(trimmed, "*") {
		return errors.New("arrays are not supported")
	}
	parts := strings.Split(strings.TrimPrefix(trimmed, "payload."), ".")
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			return errors.New("empty path segment")
		}
	}
	return nil
}

func parseFilterValue(value any) (Filter, error) {
	object, ok := value.(map[string]any)
	if !ok {
		return nil, ValidationError{}
	}
	groupKeys := 0
	if _, ok := object["all"]; ok {
		groupKeys++
	}
	if _, ok := object["any"]; ok {
		groupKeys++
	}
	if _, ok := object["not"]; ok {
		groupKeys++
	}
	hasCondition := object["path"] != nil || object["op"] != nil || object["value"] != nil
	if groupKeys+boolToInt(hasCondition) != 1 {
		return nil, ValidationError{}
	}
	if hasCondition {
		path, _ := object["path"].(string)
		op, _ := object["op"].(string)
		condition := Condition{Path: strings.TrimSpace(path), Op: strings.TrimSpace(op)}
		if value, ok := object["value"]; ok {
			condition.Value = value
		}
		return condition, nil
	}
	if rawAll, ok := object["all"]; ok {
		items, ok := rawAll.([]any)
		if !ok || len(items) == 0 {
			return nil, ValidationError{}
		}
		group := Group{All: make([]Filter, 0, len(items))}
		for _, item := range items {
			child, err := parseFilterValue(item)
			if err != nil {
				return nil, err
			}
			group.All = append(group.All, child)
		}
		return group, nil
	}
	if rawAny, ok := object["any"]; ok {
		items, ok := rawAny.([]any)
		if !ok || len(items) == 0 {
			return nil, ValidationError{}
		}
		group := Group{Any: make([]Filter, 0, len(items))}
		for _, item := range items {
			child, err := parseFilterValue(item)
			if err != nil {
				return nil, err
			}
			group.Any = append(group.Any, child)
		}
		return group, nil
	}
	child, err := parseFilterValue(object["not"])
	if err != nil {
		return nil, err
	}
	return Group{Not: child}, nil
}

func evalFilter(filter Filter, payload map[string]any) bool {
	switch typed := filter.(type) {
	case Group:
		if len(typed.All) > 0 {
			for _, child := range typed.All {
				if !evalFilter(child, payload) {
					return false
				}
			}
			return true
		}
		if len(typed.Any) > 0 {
			for _, child := range typed.Any {
				if evalFilter(child, payload) {
					return true
				}
			}
			return false
		}
		if typed.Not != nil {
			return !evalFilter(typed.Not, payload)
		}
	case Condition:
		value, ok := resolvePayloadValue(payload, typed.Path)
		return compareCondition(ok, value, typed)
	}
	return false
}

func compareCondition(found bool, actual any, condition Condition) bool {
	op := strings.TrimSpace(condition.Op)
	if op == "exists" {
		return found
	}
	if !found {
		return false
	}
	switch op {
	case "==":
		return compareEqual(actual, condition.Value)
	case "!=":
		return !compareEqual(actual, condition.Value)
	case ">", ">=", "<", "<=":
		left, lok := asNumber(actual)
		right, rok := asNumber(condition.Value)
		if !lok || !rok {
			return false
		}
		switch op {
		case ">":
			return left > right
		case ">=":
			return left >= right
		case "<":
			return left < right
		default:
			return left <= right
		}
	case "contains":
		left, lok := actual.(string)
		right, rok := condition.Value.(string)
		return lok && rok && strings.Contains(left, right)
	case "in":
		values, ok := condition.Value.([]any)
		if !ok {
			return false
		}
		for _, item := range values {
			if compareEqual(actual, item) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func resolvePayloadValue(payload map[string]any, path string) (any, bool) {
	current := any(payload)
	for _, part := range strings.Split(strings.TrimPrefix(strings.TrimSpace(path), "payload."), ".") {
		obj, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		value, ok := obj[part]
		if !ok {
			return nil, false
		}
		current = value
	}
	return current, true
}

func compareEqual(left, right any) bool {
	switch l := left.(type) {
	case string:
		r, ok := right.(string)
		return ok && l == r
	case bool:
		r, ok := right.(bool)
		return ok && l == r
	default:
		lf, lok := asNumber(left)
		rf, rok := asNumber(right)
		return lok && rok && lf == rf
	}
}

func asNumber(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

type schemaNode struct {
	Kind                 string
	Properties           map[string]schemaNode
	AllowAdditional      bool
	AdditionalProperties *schemaNode
}

func parseSchemaNode(raw json.RawMessage) (schemaNode, error) {
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
			child, ok := value.(map[string]any)
			if ok {
				node.Properties[key] = buildSchemaNode(child)
			}
		}
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

func validateAgainstSchema(filter Filter, root schemaNode, out *ValidationError) {
	switch typed := filter.(type) {
	case Group:
		for _, child := range typed.All {
			validateAgainstSchema(child, root, out)
		}
		for _, child := range typed.Any {
			validateAgainstSchema(child, root, out)
		}
		if typed.Not != nil {
			validateAgainstSchema(typed.Not, root, out)
		}
	case Condition:
		field, ok := resolveSchemaPath(root, typed.Path)
		if !ok {
			out.InvalidPaths = appendUnique(out.InvalidPaths, typed.Path)
			return
		}
		if !operatorAllowed(field.Kind, typed.Op, typed.Value) {
			out.InvalidOps = appendUnique(out.InvalidOps, typed.Path+":"+typed.Op)
		}
	}
}

func resolveSchemaPath(root schemaNode, path string) (schemaNode, bool) {
	current := root
	for _, part := range strings.Split(strings.TrimPrefix(path, "payload."), ".") {
		child, ok := current.Properties[part]
		if ok {
			current = child
			continue
		}
		if current.AdditionalProperties != nil {
			current = *current.AdditionalProperties
			continue
		}
		if current.AllowAdditional {
			return schemaNode{Kind: ""}, true
		}
		return schemaNode{}, false
	}
	return current, true
}

func operatorAllowed(kind string, op string, value any) bool {
	switch strings.TrimSpace(op) {
	case "exists":
		return true
	case "==", "!=":
		return valueMatchesKind(kind, value)
	case ">", ">=", "<", "<=":
		return isNumericKind(kind) && valueMatchesKind(kind, value)
	case "contains":
		_, ok := value.(string)
		return kind == "string" && ok
	case "in":
		values, ok := value.([]any)
		if !ok || len(values) == 0 {
			return false
		}
		for _, item := range values {
			if !valueMatchesKind(kind, item) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func valueMatchesKind(kind string, value any) bool {
	switch kind {
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "integer":
		number, ok := asNumber(value)
		return ok && number == float64(int64(number))
	case "number", "":
		_, ok := asNumber(value)
		return ok
	default:
		return false
	}
}

func isNumericKind(kind string) bool {
	return kind == "integer" || kind == "number" || kind == ""
}

func appendUnique(values []string, candidate string) []string {
	for _, value := range values {
		if value == candidate {
			return values
		}
	}
	return append(values, candidate)
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
