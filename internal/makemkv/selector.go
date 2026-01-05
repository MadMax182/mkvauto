package makemkv

import (
	"time"
)

// SelectTitles implements the intelligent title selection logic:
// - If ANY title >= 60 minutes: Rip ONLY the longest title (movie mode)
// - If ALL titles < 60 minutes: Rip ALL titles >= 18 minutes (TV episode mode)
func SelectTitles(titles []Title, movieThreshold, episodeThreshold time.Duration) []Title {
	if len(titles) == 0 {
		return nil
	}

	// Find the maximum duration
	maxDuration := time.Duration(0)
	for _, title := range titles {
		if title.Duration > maxDuration {
			maxDuration = title.Duration
		}
	}

	// Movie mode: at least one title >= movie threshold (default 60 min)
	if maxDuration >= movieThreshold {
		// Return only the longest title
		return []Title{findLongestTitle(titles)}
	}

	// TV mode: all titles < movie threshold
	// Return all titles >= episode threshold (default 18 min)
	var selected []Title
	for _, title := range titles {
		if title.Duration >= episodeThreshold {
			selected = append(selected, title)
		}
	}

	return selected
}

// findLongestTitle returns the title with the longest duration
func findLongestTitle(titles []Title) Title {
	if len(titles) == 0 {
		return Title{}
	}

	longest := titles[0]
	for _, title := range titles[1:] {
		if title.Duration > longest.Duration {
			longest = title
		}
	}

	return longest
}
