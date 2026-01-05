package makemkv

import (
	"strconv"
	"strings"
	"time"
)

type Title struct {
	ID       int
	Duration time.Duration
	Name     string
	Size     int64 // Size in bytes
	Chapters int
}

type ScanResult struct {
	Titles   []Title
	DiscName string
	DiscType string // "DVD" or "Blu-ray"
}

// ParseInfo parses the output of 'makemkvcon info disc:0'
func ParseInfo(output string) (*ScanResult, error) {
	result := &ScanResult{
		Titles: []Title{},
	}

	lines := strings.Split(output, "\n")
	titleMap := make(map[int]*Title)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse disc info
		// CINFO:2,0,"Disc Name"
		if strings.HasPrefix(line, "CINFO:2,0,\"") {
			result.DiscName = extractQuotedValue(line)
		}

		// Detect disc type
		if strings.Contains(strings.ToLower(line), "blu-ray") || strings.Contains(strings.ToLower(line), "bd-rom") {
			result.DiscType = "Blu-ray"
		} else if strings.Contains(strings.ToLower(line), "dvd") {
			if result.DiscType == "" {
				result.DiscType = "DVD"
			}
		}

		// Parse title info
		// TINFO:titleID,attributeID,source,"value"
		if strings.HasPrefix(line, "TINFO:") {
			parseTitleInfo(line, titleMap)
		}
	}

	// If disc type still not determined, default to DVD
	if result.DiscType == "" {
		result.DiscType = "DVD"
	}

	// Convert map to slice
	for _, title := range titleMap {
		if title.Duration > 0 { // Only include titles with valid duration
			result.Titles = append(result.Titles, *title)
		}
	}

	return result, nil
}

// parseTitleInfo parses TINFO lines
func parseTitleInfo(line string, titleMap map[int]*Title) {
	// Format: TINFO:titleID,attributeID,source,"value"
	parts := strings.SplitN(line[6:], ",", 4) // Skip "TINFO:"
	if len(parts) < 4 {
		return
	}

	titleID, err := strconv.Atoi(parts[0])
	if err != nil {
		return
	}

	attributeID, err := strconv.Atoi(parts[1])
	if err != nil {
		return
	}

	value := extractQuotedValue(parts[3])

	// Initialize title if not exists
	if titleMap[titleID] == nil {
		titleMap[titleID] = &Title{ID: titleID}
	}

	title := titleMap[titleID]

	switch attributeID {
	case 2: // Title name
		title.Name = value
	case 9: // Duration
		title.Duration = parseDuration(value)
	case 10: // Size in bytes
		size, _ := strconv.ParseInt(value, 10, 64)
		title.Size = size
	case 8: // Chapter count
		chapters, _ := strconv.Atoi(value)
		title.Chapters = chapters
	}
}

// extractQuotedValue extracts value from quoted string
func extractQuotedValue(s string) string {
	start := strings.Index(s, "\"")
	end := strings.LastIndex(s, "\"")
	if start != -1 && end != -1 && start < end {
		return s[start+1 : end]
	}
	return ""
}

// parseDuration parses duration string like "1:45:32" to time.Duration
func parseDuration(s string) time.Duration {
	// Format: "H:MM:SS"
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0
	}

	hours, _ := strconv.Atoi(parts[0])
	minutes, _ := strconv.Atoi(parts[1])
	seconds, _ := strconv.Atoi(parts[2])

	return time.Duration(hours)*time.Hour +
		time.Duration(minutes)*time.Minute +
		time.Duration(seconds)*time.Second
}

// ParseProgress parses progress output
// PRGV:current,total,max
func ParseProgress(line string) (current, total, max int, ok bool) {
	if !strings.HasPrefix(line, "PRGV:") {
		return 0, 0, 0, false
	}

	parts := strings.Split(line[5:], ",")
	if len(parts) < 3 {
		return 0, 0, 0, false
	}

	current, err1 := strconv.Atoi(parts[0])
	total, err2 := strconv.Atoi(parts[1])
	max, err3 := strconv.Atoi(parts[2])

	if err1 != nil || err2 != nil || err3 != nil {
		return 0, 0, 0, false
	}

	return current, total, max, true
}

// CalculatePercentage calculates percentage from progress values
func CalculatePercentage(current, total, max int) float64 {
	if max == 0 {
		return 0
	}
	return float64(current) / float64(max) * 100.0
}

// ParseStatusMessage parses PRGC/PRGT status messages
// Format: PRGC:code,id,"status message" or PRGT:code,id,"status message"
func ParseStatusMessage(line string) (status string, ok bool) {
	if !strings.HasPrefix(line, "PRGC:") && !strings.HasPrefix(line, "PRGT:") {
		return "", false
	}

	// Extract the quoted status message
	status = extractQuotedValue(line)
	if status == "" {
		return "", false
	}

	return status, true
}
