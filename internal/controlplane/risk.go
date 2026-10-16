package controlplane

import "strings"

// NormalizeRiskClass accepts the runtime R0–R3 vocabulary and textual levels.
func NormalizeRiskClass(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "r0":
		return "low"
	case "r1":
		return "medium"
	case "r2", "high_risk":
		return "high"
	case "r3":
		return "critical"
	default:
		return strings.ToLower(strings.TrimSpace(level))
	}
}

func IsHighRisk(level string) bool {
	level = NormalizeRiskClass(level)
	return level == "high" || level == "critical"
}
