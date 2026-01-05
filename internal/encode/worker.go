package encode

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type WorkerControl int

const (
	WorkerPause WorkerControl = iota
	WorkerResume
	WorkerStop
	WorkerDelete
)

type ProgressUpdate struct {
	ItemID   string
	Progress float64
}

type Worker struct {
	queue           *Queue
	handbrake       *HandBrake
	progressCh      chan<- ProgressUpdate
	controlCh       <-chan WorkerControl
	logCh           chan<- string
	paused          bool
	shouldDeleteCurrent bool
}

func NewWorker(queue *Queue, handbrake *HandBrake, progressCh chan<- ProgressUpdate, controlCh <-chan WorkerControl, logCh chan<- string) *Worker {
	return &Worker{
		queue:      queue,
		handbrake:  handbrake,
		progressCh: progressCh,
		controlCh:  controlCh,
		logCh:      logCh,
		paused:     false,
	}
}

// Run starts the worker loop
func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case ctrl := <-w.controlCh:
			w.handleControl(ctrl)

		case <-ticker.C:
			if w.paused {
				continue
			}

			// Get next queued item
			item := w.queue.GetNext()
			if item == nil {
				continue
			}

			// Process the item
			w.encodeItem(ctx, item)
		}
	}
}

// handleControl handles pause/resume/stop commands
func (w *Worker) handleControl(ctrl WorkerControl) {
	switch ctrl {
	case WorkerPause:
		w.paused = true
		w.handbrake.Pause()
	case WorkerResume:
		w.paused = false
		w.handbrake.Resume()
	case WorkerStop:
		w.shouldDeleteCurrent = false
		w.handbrake.Cancel()
	case WorkerDelete:
		w.shouldDeleteCurrent = true
		w.handbrake.Cancel()
	}
}

// encodeItem encodes a single item
func (w *Worker) encodeItem(ctx context.Context, item *QueueItem) {
	// Mark as encoding
	if err := w.queue.SetStatus(item.ID, StatusEncoding); err != nil {
		fmt.Printf("Failed to set encoding status: %v\n", err)
		return
	}

	// Send initial progress update to set currentEncode in UI
	w.progressCh <- ProgressUpdate{
		ItemID:   item.ID,
		Progress: 0,
	}

	// Create progress channel for this item
	progressCh := make(chan float64, 10)

	// Forward progress updates
	go func() {
		for progress := range progressCh {
			w.queue.UpdateProgress(item.ID, progress)
			w.progressCh <- ProgressUpdate{
				ItemID:   item.ID,
				Progress: progress,
			}
		}
	}()

	// Monitor control channel during encoding
	encodeDone := make(chan error, 1)
	go func() {
		encodeDone <- w.handbrake.Encode(ctx, item, progressCh, w.logCh)
	}()

	// Wait for encoding to complete or control signal
	var err error
	for {
		select {
		case err = <-encodeDone:
			goto handleResult
		case ctrl := <-w.controlCh:
			w.handleControl(ctrl)
		}
	}

handleResult:
	close(progressCh)

	if err != nil {
		// Check if it was cancelled (process killed)
		errStr := err.Error()
		if strings.Contains(errStr, "killed") || strings.Contains(errStr, "signal") {
			// Check if we should delete or just fail
			if w.shouldDeleteCurrent {
				// Delete from queue
				w.queue.Remove(item.ID)
				if w.logCh != nil {
					w.logCh <- fmt.Sprintf("Encoding cancelled and removed: %s", item.TitleName)
				}
				w.shouldDeleteCurrent = false
				return
			} else {
				// Mark as failed (can be retried)
				w.queue.Fail(item.ID, fmt.Errorf("cancelled by user"))
				if w.logCh != nil {
					w.logCh <- fmt.Sprintf("Encoding cancelled: %s", item.TitleName)
				}
				return
			}
		}

		// Real failure - mark as failed
		w.queue.Fail(item.ID, err)
		fmt.Printf("Encoding failed for %s: %v\n", item.TitleName, err)
		return
	}

	// Mark as complete
	w.queue.Complete(item.ID)
}
