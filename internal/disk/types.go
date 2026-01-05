package disk

import (
	"strings"
)

type DiscType int

const (
	DiscTypeDVD DiscType = iota
	DiscTypeBluRay
)

func (dt DiscType) String() string {
	switch dt {
	case DiscTypeDVD:
		return "DVD"
	case DiscTypeBluRay:
		return "Blu-ray"
	default:
		return "Unknown"
	}
}

type DetectedDisc struct {
	Device   string
	Name     string
	DiscType DiscType
}

// DetectDiscType determines if a disc is DVD or Blu-ray
// This can be called after MakeMKV has scanned the disc
func DetectDiscTypeFromInfo(info string) DiscType {
	// Look for indicators in the MakeMKV output
	infoLower := strings.ToLower(info)
	if strings.Contains(infoLower, "blu-ray") || strings.Contains(infoLower, "bd") {
		return DiscTypeBluRay
	}
	return DiscTypeDVD
}

// ParseDiscName extracts a clean disc name from MakeMKV info
func ParseDiscName(info string) string {
	// This will be implemented based on MakeMKV output format
	// For now, return a placeholder
	lines := strings.Split(info, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "CINFO:2,0,\"") {
			// Extract disc name from MakeMKV output
			// Format: CINFO:2,0,"Disc Name"
			start := strings.Index(line, "\"")
			end := strings.LastIndex(line, "\"")
			if start != -1 && end != -1 && start < end {
				return line[start+1 : end]
			}
		}
	}
	return "Unknown_Disc"
}

// SanitizeFilename removes or replaces characters that are problematic in filenames
func SanitizeFilename(name string) string {
	// Replace problematic characters
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
		" ", "_",
	)
	sanitized := replacer.Replace(name)

	// Remove any leading/trailing underscores or dots
	sanitized = strings.Trim(sanitized, "_.")

	if sanitized == "" {
		return "Unnamed_Disc"
	}

	return sanitized
}
