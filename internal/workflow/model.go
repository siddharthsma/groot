package workflow

import (
	"encoding/json"

	specpkg "groot/internal/workflow/spec"
)

const (
	StatusDraft     = specpkg.StatusDraft
	StatusPublished = specpkg.StatusPublished
	StatusArchived  = specpkg.StatusArchived

	NodeTypeTrigger   = specpkg.NodeTypeTrigger
	NodeTypeAction    = specpkg.NodeTypeAction
	NodeTypeCondition = specpkg.NodeTypeCondition
	NodeTypeAgent     = specpkg.NodeTypeAgent
	NodeTypeWait      = specpkg.NodeTypeWait
	NodeTypeEnd       = specpkg.NodeTypeEnd

	RunStatusRunning   = specpkg.RunStatusRunning
	RunStatusWaiting   = specpkg.RunStatusWaiting
	RunStatusSucceeded = specpkg.RunStatusSucceeded
	RunStatusFailed    = specpkg.RunStatusFailed
	RunStatusTimedOut  = specpkg.RunStatusTimedOut
	RunStatusPartial   = specpkg.RunStatusPartial
	RunStatusCancelled = specpkg.RunStatusCancelled

	RunStepStatusPending   = specpkg.RunStepStatusPending
	RunStepStatusRunning   = specpkg.RunStepStatusRunning
	RunStepStatusWaiting   = specpkg.RunStepStatusWaiting
	RunStepStatusSucceeded = specpkg.RunStepStatusSucceeded
	RunStepStatusFailed    = specpkg.RunStepStatusFailed
	RunStepStatusSkipped   = specpkg.RunStepStatusSkipped
	RunStepStatusTimedOut  = specpkg.RunStepStatusTimedOut

	RunWaitStatusWaiting   = specpkg.RunWaitStatusWaiting
	RunWaitStatusMatched   = specpkg.RunWaitStatusMatched
	RunWaitStatusTimedOut  = specpkg.RunWaitStatusTimedOut
	RunWaitStatusCancelled = specpkg.RunWaitStatusCancelled
)

var (
	ErrInvalidWorkflowName   = specpkg.ErrInvalidWorkflowName
	ErrDuplicateWorkflowName = specpkg.ErrDuplicateWorkflowName
	ErrWorkflowNotFound      = specpkg.ErrWorkflowNotFound
	ErrVersionNotFound       = specpkg.ErrVersionNotFound
	ErrInvalidDefinition     = specpkg.ErrInvalidDefinition
)

type Workflow = specpkg.Workflow
type WorkflowRecord = specpkg.WorkflowRecord
type Version = specpkg.Version
type VersionRecord = specpkg.VersionRecord
type Definition = specpkg.Definition
type Node = specpkg.Node
type Position = specpkg.Position
type Edge = specpkg.Edge
type TriggerConfig = specpkg.TriggerConfig
type ActionConfig = specpkg.ActionConfig
type ConditionConfig = specpkg.ConditionConfig
type AgentConfig = specpkg.AgentConfig
type WaitConfig = specpkg.WaitConfig
type EndConfig = specpkg.EndConfig
type ValidationIssue = specpkg.ValidationIssue
type ValidationFailedError = specpkg.ValidationFailedError
type EntryBinding = specpkg.EntryBinding
type Artifacts = specpkg.Artifacts
type SubscriptionArtifact = specpkg.SubscriptionArtifact
type ArtifactsSummary = specpkg.ArtifactsSummary
type Run = specpkg.Run
type RunRecord = specpkg.RunRecord
type RunStep = specpkg.RunStep
type RunStepRecord = specpkg.RunStepRecord
type RunWait = specpkg.RunWait
type RunWaitRecord = specpkg.RunWaitRecord

func NormalizeName(name string) string {
	return specpkg.NormalizeName(name)
}

func ParseDefinition(raw json.RawMessage) (Definition, error) {
	return specpkg.ParseDefinition(raw)
}

func NormalizeDefinitionJSON(raw json.RawMessage) (json.RawMessage, Definition, error) {
	return specpkg.NormalizeDefinitionJSON(raw)
}
