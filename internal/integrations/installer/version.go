package installer

import (
	"fmt"
	"strconv"
	"strings"
)

type semver struct {
	major int
	minor int
	patch int
}

func parseSemver(value string) (semver, error) {
	parts := strings.Split(strings.TrimSpace(value), ".")
	if len(parts) != 3 {
		return semver{}, fmt.Errorf("invalid semver %q", value)
	}
	var parsed [3]int
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return semver{}, fmt.Errorf("invalid semver %q", value)
		}
		parsed[i] = n
	}
	return semver{major: parsed[0], minor: parsed[1], patch: parsed[2]}, nil
}

func compareSemver(left, right semver) int {
	switch {
	case left.major != right.major:
		return left.major - right.major
	case left.minor != right.minor:
		return left.minor - right.minor
	default:
		return left.patch - right.patch
	}
}

func validateVersionConstraint(constraint string, currentVersion string) error {
	constraint = strings.TrimSpace(constraint)
	if constraint == "" {
		return nil
	}
	if strings.TrimSpace(currentVersion) == "" {
		currentVersion = "dev"
	}
	if currentVersion == "dev" {
		return fmt.Errorf("version-constrained packages cannot be installed on build version dev")
	}
	current, err := parseSemver(currentVersion)
	if err != nil {
		return fmt.Errorf("parse current groot version: %w", err)
	}
	switch {
	case strings.HasPrefix(constraint, ">="):
		minimum, err := parseSemver(strings.TrimSpace(strings.TrimPrefix(constraint, ">=")))
		if err != nil {
			return fmt.Errorf("parse groot_version constraint: %w", err)
		}
		if compareSemver(current, minimum) < 0 {
			return fmt.Errorf("integration requires groot_version %s", constraint)
		}
		return nil
	default:
		exact, err := parseSemver(strings.TrimPrefix(constraint, "="))
		if err != nil {
			return fmt.Errorf("parse groot_version constraint: %w", err)
		}
		if compareSemver(current, exact) != 0 {
			return fmt.Errorf("integration requires groot_version %s", constraint)
		}
		return nil
	}
}
