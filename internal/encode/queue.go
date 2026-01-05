package encode

import (
	"sync"
	"time"

	"github.com/mmzim/mkvauto/internal/disk"
)

type ItemStatus int

const (
	StatusQueued ItemStatus = iota
	StatusEncoding
	StatusPaused
	StatusComplete
	StatusFailed
)

func (s ItemStatus) String() string {
	switch s {
	case StatusQueued:
		return "Queued"
	case StatusEncoding:
		return "Encoding"
	case StatusPaused:
		return "Paused"
	case StatusComplete:
		return "Complete"
	case StatusFailed:
		return "Failed"
	default:
		return "Unknown"
	}
}

type QueueItem struct {
	ID          string           `json:"id"`
	SourcePath  string           `json:"source_path"`
	DestPath    string           `json:"dest_path"`
	DiscType    disk.DiscType    `json:"disc_type"`
	DiscName    string           `json:"disc_name"`
	TitleName   string           `json:"title_name"`
	Status      ItemStatus       `json:"status"`
	Progress    float64          `json:"progress"`
	CreatedAt   time.Time        `json:"created_at"`
	StartedAt   *time.Time       `json:"started_at,omitempty"`
	CompletedAt *time.Time       `json:"completed_at,omitempty"`
	Error       string           `json:"error,omitempty"`
}

type Queue struct {
	items       []*QueueItem
	mu          sync.RWMutex
	persistence *StatePersistence
}

func NewQueue(statePath string) *Queue {
	return &Queue{
		items:       make([]*QueueItem, 0),
		persistence: NewStatePersistence(statePath),
	}
}

// LoadState loads the queue from disk
func (q *Queue) LoadState() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	items, err := q.persistence.Load()
	if err != nil {
		// If file doesn't exist, that's okay, start with empty queue
		return nil
	}

	q.items = items

	// Reset any items stuck in "encoding" state from interrupted sessions
	for _, item := range q.items {
		if item.Status == StatusEncoding {
			item.Status = StatusQueued
			item.Progress = 0
			item.StartedAt = nil
		}
	}

	// Save the cleaned state
	q.persistence.Save(q.items)

	return nil
}

// SaveState saves the queue to disk
func (q *Queue) SaveState() error {
	q.mu.RLock()
	defer q.mu.RUnlock()

	return q.persistence.Save(q.items)
}

// Add adds a new item to the queue
func (q *Queue) Add(item *QueueItem) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.items = append(q.items, item)

	// Save to disk
	return q.persistence.Save(q.items)
}

// HasSourcePath checks if an item with the given source path already exists in the queue
func (q *Queue) HasSourcePath(sourcePath string) bool {
	q.mu.RLock()
	defer q.mu.RUnlock()

	for _, item := range q.items {
		if item.SourcePath == sourcePath {
			return true
		}
	}
	return false
}

// GetNext returns the next queued item, or nil if none available
func (q *Queue) GetNext() *QueueItem {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, item := range q.items {
		if item.Status == StatusQueued {
			return item
		}
	}

	return nil
}

// UpdateProgress updates the progress of an item
func (q *Queue) UpdateProgress(id string, progress float64) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, item := range q.items {
		if item.ID == id {
			item.Progress = progress
			return q.persistence.Save(q.items)
		}
	}

	return nil
}

// SetStatus sets the status of an item
func (q *Queue) SetStatus(id string, status ItemStatus) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, item := range q.items {
		if item.ID == id {
			item.Status = status

			now := time.Now()
			switch status {
			case StatusEncoding:
				item.StartedAt = &now
			case StatusComplete, StatusFailed:
				item.CompletedAt = &now
			}

			return q.persistence.Save(q.items)
		}
	}

	return nil
}

// Complete marks an item as complete
func (q *Queue) Complete(id string) error {
	return q.SetStatus(id, StatusComplete)
}

// Fail marks an item as failed
func (q *Queue) Fail(id string, err error) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, item := range q.items {
		if item.ID == id {
			item.Status = StatusFailed
			item.Error = err.Error()
			now := time.Now()
			item.CompletedAt = &now
			return q.persistence.Save(q.items)
		}
	}

	return nil
}

// Pause pauses an encoding item
func (q *Queue) Pause(id string) error {
	return q.SetStatus(id, StatusPaused)
}

// Resume resumes a paused item
func (q *Queue) Resume(id string) error {
	return q.SetStatus(id, StatusQueued)
}

// GetAll returns all items (for UI display)
func (q *Queue) GetAll() []*QueueItem {
	q.mu.RLock()
	defer q.mu.RUnlock()

	// Return a copy
	items := make([]*QueueItem, len(q.items))
	copy(items, q.items)
	return items
}

// GetCurrent returns the currently encoding item, if any
func (q *Queue) GetCurrent() *QueueItem {
	q.mu.RLock()
	defer q.mu.RUnlock()

	for _, item := range q.items {
		if item.Status == StatusEncoding {
			return item
		}
	}

	return nil
}

// ClearCompleted removes completed and failed items from the queue
func (q *Queue) ClearCompleted() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	filtered := make([]*QueueItem, 0)
	for _, item := range q.items {
		if item.Status != StatusComplete && item.Status != StatusFailed {
			filtered = append(filtered, item)
		}
	}

	q.items = filtered
	return q.persistence.Save(q.items)
}

// RetryFailed resets all failed and stuck encoding items to queued status for retry
func (q *Queue) RetryFailed() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, item := range q.items {
		// Reset failed items and stuck encoding items (from interrupted sessions)
		if item.Status == StatusFailed || item.Status == StatusEncoding {
			item.Status = StatusQueued
			item.Progress = 0
			item.Error = ""
			item.StartedAt = nil
			item.CompletedAt = nil
		}
	}

	return q.persistence.Save(q.items)
}

// Remove removes an item from the queue by ID
func (q *Queue) Remove(id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	filtered := make([]*QueueItem, 0)
	for _, item := range q.items {
		if item.ID != id {
			filtered = append(filtered, item)
		}
	}

	q.items = filtered
	return q.persistence.Save(q.items)
}
