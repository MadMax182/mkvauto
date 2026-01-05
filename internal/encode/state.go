package encode

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type StatePersistence struct {
	filePath string
}

func NewStatePersistence(filePath string) *StatePersistence {
	return &StatePersistence{
		filePath: filePath,
	}
}

// Save saves the queue items to disk atomically
func (sp *StatePersistence) Save(items []*QueueItem) error {
	// Ensure directory exists
	dir := filepath.Dir(sp.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal queue state: %w", err)
	}

	// Write to temporary file first
	tmpPath := sp.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp state file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, sp.filePath); err != nil {
		return fmt.Errorf("failed to rename state file: %w", err)
	}

	return nil
}

// Load loads the queue items from disk
func (sp *StatePersistence) Load() ([]*QueueItem, error) {
	data, err := os.ReadFile(sp.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist yet, return empty queue
			return make([]*QueueItem, 0), nil
		}
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	var items []*QueueItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("failed to unmarshal queue state: %w", err)
	}

	return items, nil
}

// Delete removes the state file
func (sp *StatePersistence) Delete() error {
	if err := os.Remove(sp.filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete state file: %w", err)
	}
	return nil
}
