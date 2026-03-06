package schemas

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

var fullNamePattern = regexp.MustCompile(`^([a-z0-9._-]+)\.v([1-9][0-9]*)$`)

type Schema struct {
	ID         uuid.UUID
	EventType  string
	Version    int
	FullName   string
	Source     string
	SourceKind string
	SchemaJSON json.RawMessage
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type Record struct {
	ID         uuid.UUID
	EventType  string
	Version    int
	FullName   string
	Source     string
	SourceKind string
	SchemaJSON json.RawMessage
}

type Spec struct {
	EventType  string
	Version    int
	Source     string
	SourceKind string
	SchemaJSON json.RawMessage
}

type Bundle struct {
	Name    string
	Schemas []Spec
}

type ValidationMode string

const (
	ValidationModeOff    ValidationMode = "off"
	ValidationModeWarn   ValidationMode = "warn"
	ValidationModeReject ValidationMode = "reject"
)

type Config struct {
	ValidationMode   ValidationMode
	RegistrationMode string
	MaxPayloadBytes  int
}

type RejectError struct {
	reason string
}

func (e RejectError) Error() string {
	return e.reason
}

type TemplatePathError struct {
	Path string
}

func (e TemplatePathError) Error() string {
	return fmt.Sprintf("invalid template path: %s", e.Path)
}

func FullName(eventType string, version int) string {
	return fmt.Sprintf("%s.v%d", strings.TrimSpace(eventType), version)
}

func ParseFullName(fullName string) (string, int, bool) {
	matches := fullNamePattern.FindStringSubmatch(strings.TrimSpace(fullName))
	if len(matches) != 3 {
		return "", 0, false
	}
	version, err := strconv.Atoi(matches[2])
	if err != nil || version < 1 {
		return "", 0, false
	}
	return matches[1], version, true
}

func NormalizeValidationMode(value string) ValidationMode {
	switch ValidationMode(strings.TrimSpace(value)) {
	case ValidationModeOff:
		return ValidationModeOff
	case ValidationModeReject:
		return ValidationModeReject
	default:
		return ValidationModeWarn
	}
}
