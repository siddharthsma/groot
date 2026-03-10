package compiler

import specpkg "groot/internal/workflow/spec"

func Validate(definition specpkg.Definition) []specpkg.ValidationIssue {
	issues := make([]specpkg.ValidationIssue, 0)
	triggerCount := 0
	for _, node := range definition.Nodes {
		switch node.Type {
		case specpkg.NodeTypeTrigger:
			triggerCount++
		case specpkg.NodeTypeAction, specpkg.NodeTypeCondition, specpkg.NodeTypeAgent, specpkg.NodeTypeWait, specpkg.NodeTypeEnd:
		default:
			issues = append(issues, specpkg.ValidationIssue{
				Code:    "unsupported_node_type",
				Path:    "nodes." + node.ID,
				Message: "node type is not compileable",
			})
		}
	}
	if triggerCount == 0 {
		issues = append(issues, specpkg.ValidationIssue{
			Code:    "missing_trigger",
			Path:    "nodes",
			Message: "at least one trigger node is required",
		})
	}
	return issues
}
