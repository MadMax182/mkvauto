package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mmzim/mkvauto/internal/encode"
)

type RipState int

const (
	StateWaiting RipState = iota
	StateScanning
	StateSelectingTitles
	StateRipping
	StateComplete
	StateError
)

type DiskInfo struct {
	Name     string
	DiscType string
}

// Title represents a disc title for selection
type Title struct {
	ID       int
	Name     string
	Duration string
	Size     string
	Selected bool
}

// Messages for bubbletea
type DiskInsertedMsg struct{}
type ScanCompleteMsg struct {
	Info DiskInfo
}
type StatusUpdateMsg struct {
	Status string
}
type ShowTitleSelectionMsg struct {
	Titles []Title
}
type TitlesSelectedMsg struct {
	SelectedIDs []int
}
type RipProgressMsg struct {
	Progress     float64
	CurrentTitle int
	TotalTitles  int
}
type RipCompleteMsg struct{}
type EncodeProgressMsg struct {
	ItemID   string
	Progress float64
}
type EncodeCompleteMsg struct {
	ItemID string
}
type QueueUpdateMsg struct{}
type ErrorMsg struct {
	Err error
}
type LogMsg struct {
	Line string
}
type CancelAndEjectMsg struct{}
type ScanForMissingMsg struct{}

type Model struct {
	// Ripping state
	ripState      RipState
	ripStatus     string // Current operation status (e.g., "Opening disc...", "Processing titles...")
	diskInfo      DiskInfo
	ripProgress   float64
	currentTitle  int
	totalTitles   int
	ripPaused     bool
	ripStartTime  time.Time
	ripETA        string

	// Title selection
	availableTitles []Title
	selectedCursor  int
	titleSelectionCh chan<- []int

	// Encoding state
	encodeQueue      *encode.Queue
	currentEncode    *encode.QueueItem
	encodePaused     bool
	encodeStartTime  time.Time
	encodeETA        string

	// UI components
	ripProgressBar    progress.Model
	encodeProgressBar progress.Model

	// Controls
	workerControl chan encode.WorkerControl
	cancelRipCh   chan<- struct{}
	scanRequestCh chan<- struct{}

	// Logs
	showLogs bool
	logLines []string
	maxLogs  int

	// Config
	outputDir string

	// Error
	err error

	// Window size
	width  int
	height int
}

func NewModel(queue *encode.Queue, workerControl chan encode.WorkerControl, titleSelectionCh chan<- []int, outputDir string, cancelRipCh chan<- struct{}, scanRequestCh chan<- struct{}) Model {
	return Model{
		ripState:          StateWaiting,
		encodeQueue:       queue,
		workerControl:     workerControl,
		titleSelectionCh:  titleSelectionCh,
		cancelRipCh:       cancelRipCh,
		scanRequestCh:     scanRequestCh,
		ripProgressBar:    progress.New(progress.WithDefaultGradient()),
		encodeProgressBar: progress.New(progress.WithDefaultGradient()),
		showLogs:          false,
		logLines:          make([]string, 0),
		maxLogs:           500,
		outputDir:         outputDir,
		width:             80,
		height:            24,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyPress(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case DiskInsertedMsg:
		m.ripState = StateScanning
		m.ripStatus = "Initializing scan..."
		return m, nil

	case StatusUpdateMsg:
		m.ripStatus = msg.Status
		return m, nil

	case ScanCompleteMsg:
		m.diskInfo = msg.Info
		m.ripState = StateRipping
		m.ripStatus = ""
		return m, nil

	case ShowTitleSelectionMsg:
		m.availableTitles = msg.Titles
		m.selectedCursor = 0
		m.ripState = StateSelectingTitles
		return m, nil

	case RipProgressMsg:
		// Initialize start time if this is the first progress update
		if m.ripProgress == 0 && msg.Progress > 0 {
			m.ripStartTime = time.Now()
		}

		m.ripProgress = msg.Progress
		m.currentTitle = msg.CurrentTitle
		m.totalTitles = msg.TotalTitles

		// Calculate ETA
		if msg.Progress > 0 && msg.Progress < 100 {
			elapsed := time.Since(m.ripStartTime).Seconds()
			totalEstimated := elapsed / (msg.Progress / 100.0)
			remaining := totalEstimated - elapsed

			if remaining > 0 {
				remainingDuration := time.Duration(remaining) * time.Second
				hours := int(remainingDuration.Hours())
				minutes := int(remainingDuration.Minutes()) % 60
				seconds := int(remainingDuration.Seconds()) % 60

				if hours > 0 {
					m.ripETA = fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
				} else if minutes > 0 {
					m.ripETA = fmt.Sprintf("%dm %ds", minutes, seconds)
				} else {
					m.ripETA = fmt.Sprintf("%ds", seconds)
				}
			}
		} else if msg.Progress >= 100 {
			m.ripETA = "Complete"
		}

		return m, nil

	case RipCompleteMsg:
		m.ripState = StateComplete
		m.ripProgress = 100.0
		return m, nil

	case EncodeProgressMsg:
		// Initialize start time if this is the first progress update
		if m.currentEncode == nil || m.currentEncode.Progress == 0 && msg.Progress > 0 {
			m.encodeStartTime = time.Now()
		}

		// Update progress in queue
		m.encodeQueue.UpdateProgress(msg.ItemID, msg.Progress)
		m.currentEncode = m.encodeQueue.GetCurrent()

		// Calculate ETA
		if m.currentEncode != nil && msg.Progress > 0 && msg.Progress < 100 {
			elapsed := time.Since(m.encodeStartTime).Seconds()
			totalEstimated := elapsed / (msg.Progress / 100.0)
			remaining := totalEstimated - elapsed

			if remaining > 0 {
				remainingDuration := time.Duration(remaining) * time.Second
				hours := int(remainingDuration.Hours())
				minutes := int(remainingDuration.Minutes()) % 60
				seconds := int(remainingDuration.Seconds()) % 60

				if hours > 0 {
					m.encodeETA = fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
				} else if minutes > 0 {
					m.encodeETA = fmt.Sprintf("%dm %ds", minutes, seconds)
				} else {
					m.encodeETA = fmt.Sprintf("%ds", seconds)
				}
			}
		} else if m.currentEncode != nil && msg.Progress >= 100 {
			m.encodeETA = "Complete"
		}

		return m, nil

	case EncodeCompleteMsg:
		m.currentEncode = nil
		return m, nil

	case ErrorMsg:
		m.err = msg.Err
		m.ripState = StateError
		return m, nil

	case QueueUpdateMsg:
		// Refresh queue display
		return m, nil

	case LogMsg:
		// Add log line
		m.logLines = append(m.logLines, msg.Line)
		// Keep only last maxLogs lines
		if len(m.logLines) > m.maxLogs {
			m.logLines = m.logLines[len(m.logLines)-m.maxLogs:]
		}
		return m, nil

	case CancelAndEjectMsg:
		// Send cancel signal
		select {
		case m.cancelRipCh <- struct{}{}:
		default:
		}

		// Reset to waiting state
		m.ripState = StateWaiting
		m.ripStatus = ""
		m.ripProgress = 0
		m.currentTitle = 0
		m.totalTitles = 0
		m.ripETA = ""
		return m, nil

	case ScanForMissingMsg:
		// Trigger scan for missing encodes
		select {
		case m.scanRequestCh <- struct{}{}:
		default:
		}
		return m, nil
	}

	return m, nil
}

func (m Model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle title selection mode
	if m.ripState == StateSelectingTitles {
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "up", "k":
			if m.selectedCursor > 0 {
				m.selectedCursor--
			}
			return m, nil

		case "down", "j":
			if m.selectedCursor < len(m.availableTitles)-1 {
				m.selectedCursor++
			}
			return m, nil

		case " ": // Space - toggle selection
			if m.selectedCursor < len(m.availableTitles) {
				m.availableTitles[m.selectedCursor].Selected = !m.availableTitles[m.selectedCursor].Selected
			}
			return m, nil

		case "a": // Select all
			for i := range m.availableTitles {
				m.availableTitles[i].Selected = true
			}
			return m, nil

		case "n": // Select none
			for i := range m.availableTitles {
				m.availableTitles[i].Selected = false
			}
			return m, nil

		case "enter":
			// Confirm selection
			var selectedIDs []int
			for _, title := range m.availableTitles {
				if title.Selected {
					selectedIDs = append(selectedIDs, title.ID)
				}
			}

			if len(selectedIDs) > 0 {
				// Send selected titles back to app
				m.titleSelectionCh <- selectedIDs
				m.ripState = StateRipping
			}
			return m, nil
		}
		return m, nil
	}

	// Normal mode key handling
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "x", "e":
		// Cancel current operation and eject disc
		if m.ripState == StateScanning || m.ripState == StateRipping || m.ripState == StateSelectingTitles {
			return m, func() tea.Msg {
				return CancelAndEjectMsg{}
			}
		}
		return m, nil

	case "p":
		// Pause ripping
		m.ripPaused = true
		return m, nil

	case "r":
		// Resume ripping
		m.ripPaused = false
		return m, nil

	case " ": // Space
		// Toggle encode pause/resume
		if m.currentEncode != nil {
			if m.encodePaused {
				m.workerControl <- encode.WorkerResume
				m.encodePaused = false
			} else {
				m.workerControl <- encode.WorkerPause
				m.encodePaused = true
			}
		}
		return m, nil

	case "s":
		// Stop/cancel current encode (keeps in queue as failed)
		if m.currentEncode != nil {
			m.workerControl <- encode.WorkerStop
			m.encodePaused = false
		}
		return m, nil

	case "d":
		// Delete current encode (stop and remove from queue)
		if m.currentEncode != nil {
			m.workerControl <- encode.WorkerDelete
			m.encodePaused = false
		}
		return m, nil

	case "c":
		// Clear completed items
		m.encodeQueue.ClearCompleted()
		return m, nil

	case "l":
		// Toggle log view
		m.showLogs = !m.showLogs
		return m, nil

	case "t":
		// Retry failed items
		m.encodeQueue.RetryFailed()
		return m, nil

	case "a":
		// Scan for raw files missing encoded versions
		return m, func() tea.Msg {
			return ScanForMissingMsg{}
		}
	}

	return m, nil
}

func (m Model) View() string {
	var sections []string

	// Header
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")).
		Padding(0, 1)

	sections = append(sections, headerStyle.Render("MakeMKV Auto-Ripper"))
	sections = append(sections, strings.Repeat("─", m.width))

	// Ripping section
	sections = append(sections, m.renderRippingSection())
	sections = append(sections, strings.Repeat("─", m.width))

	// Encoding section
	sections = append(sections, m.renderEncodingSection())
	sections = append(sections, strings.Repeat("─", m.width))

	// Controls
	sections = append(sections, m.renderControls())

	// Log section (shown at bottom if enabled, greyed out)
	if m.showLogs {
		// Count lines used so far
		currentContent := strings.Join(sections, "\n")
		usedLines := strings.Count(currentContent, "\n") + 1

		sections = append(sections, m.renderLogSection(usedLines))
	}

	return strings.Join(sections, "\n")
}

func (m Model) renderRippingSection() string {
	title := lipgloss.NewStyle().Bold(true).Render("RIPPING")

	var lines []string
	lines = append(lines, title)

	switch m.ripState {
	case StateWaiting:
		lines = append(lines, "Waiting for disc insertion...")

	case StateScanning:
		if m.ripStatus != "" {
			lines = append(lines, m.ripStatus)
		} else {
			lines = append(lines, "Scanning disc, please wait...")
		}

	case StateSelectingTitles:
		lines = append(lines, fmt.Sprintf("Disc: %s (%s)", m.diskInfo.Name, m.diskInfo.DiscType))
		lines = append(lines, "")
		lines = append(lines, "No titles matched automatic selection criteria.")
		lines = append(lines, "Please select titles to rip:")
		lines = append(lines, "")

		// Show titles with selection checkboxes
		for i, t := range m.availableTitles {
			checkbox := "[ ]"
			if t.Selected {
				checkbox = "[✓]"
			}

			cursor := "  "
			if i == m.selectedCursor {
				cursor = "→ "
			}

			titleLine := fmt.Sprintf("%s%s Title %d: %s - %s (%s)",
				cursor, checkbox, t.ID, t.Name, t.Duration, t.Size)

			if i == m.selectedCursor {
				highlightStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
				titleLine = highlightStyle.Render(titleLine)
			}

			lines = append(lines, titleLine)
		}

		lines = append(lines, "")
		lines = append(lines, "[↑↓] Navigate  [Space] Toggle  [A] Select All  [N] None  [Enter] Confirm")

	case StateRipping:
		lines = append(lines, fmt.Sprintf("Disc: %s (%s)", m.diskInfo.Name, m.diskInfo.DiscType))
		lines = append(lines, fmt.Sprintf("Output: %s", m.outputDir))

		if m.ripStatus != "" {
			lines = append(lines, fmt.Sprintf("Status: %s", m.ripStatus))
		} else {
			lines = append(lines, fmt.Sprintf("Status: Ripping title %d of %d (%.1f%%)", m.currentTitle, m.totalTitles, m.ripProgress))
		}

		if m.ripETA != "" {
			lines = append(lines, fmt.Sprintf("ETA: %s", m.ripETA))
		}
		lines = append(lines, m.ripProgressBar.ViewAs(m.ripProgress/100.0))
		if m.ripPaused {
			lines = append(lines, "[PAUSED] Press R to resume")
		} else {
			lines = append(lines, "[P] Pause  [R] Resume")
		}

	case StateComplete:
		lines = append(lines, "Rip complete! Disc ejected.")
		lines = append(lines, "Waiting for next disc...")

	case StateError:
		errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
		lines = append(lines, errorStyle.Render(fmt.Sprintf("Error: %v", m.err)))
	}

	return strings.Join(lines, "\n")
}

func (m Model) renderEncodingSection() string {
	title := lipgloss.NewStyle().Bold(true).Render("ENCODING QUEUE")

	queueItems := m.encodeQueue.GetAll()
	queueSize := len(queueItems)

	var lines []string
	lines = append(lines, fmt.Sprintf("%s (%d items)", title, queueSize))

	// Only show current encode if it's actually still encoding
	if m.currentEncode != nil && m.currentEncode.Status == encode.StatusEncoding {
		lines = append(lines, fmt.Sprintf("▶ Current: %s (%s/AV1)", m.currentEncode.TitleName, m.currentEncode.DiscType))
		if m.encodeETA != "" {
			lines = append(lines, fmt.Sprintf("  ETA: %s", m.encodeETA))
		}
		lines = append(lines, fmt.Sprintf("  %s", m.encodeProgressBar.ViewAs(m.currentEncode.Progress/100.0)))

		if m.encodePaused {
			lines = append(lines, "  [PAUSED] Press Space to resume")
		} else {
			lines = append(lines, "  [Space] Pause/Resume")
		}
		lines = append(lines, "")
	}

	// Show all items (except currently encoding one)
	queuedCount := 0
	completedCount := 0
	failedCount := 0

	for _, item := range queueItems {
		// Skip if this is the current encode (already shown above)
		if m.currentEncode != nil && item.ID == m.currentEncode.ID && item.Status == encode.StatusEncoding {
			continue
		}

		switch item.Status {
		case encode.StatusQueued:
			lines = append(lines, fmt.Sprintf("⏸ Queued: %s (%s)", item.TitleName, item.DiscType))
			queuedCount++
		case encode.StatusComplete:
			lines = append(lines, fmt.Sprintf("✓ Complete: %s (%s)", item.TitleName, item.DiscType))
			completedCount++
		case encode.StatusFailed:
			errorMsg := item.Error
			if len(errorMsg) > 40 {
				errorMsg = errorMsg[:37] + "..."
			}
			lines = append(lines, fmt.Sprintf("✗ Failed: %s - %s", item.TitleName, errorMsg))
			failedCount++
		}
	}

	// Show "no items" only if queue is completely empty
	hasCurrentEncode := m.currentEncode != nil && m.currentEncode.Status == encode.StatusEncoding
	if queuedCount == 0 && completedCount == 0 && failedCount == 0 && !hasCurrentEncode {
		lines = append(lines, "No items in queue")
	}

	return strings.Join(lines, "\n")
}

func (m Model) renderLogSection(usedLines int) string {
	if len(m.logLines) == 0 {
		return ""
	}

	// Slightly greyed out style (brighter than before)
	logStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))

	var lines []string

	// Calculate available space for logs to fill to bottom of screen
	// Leave 1 line margin at bottom
	availableLogLines := m.height - usedLines - 1

	// Ensure minimum of 5 lines
	if availableLogLines < 5 {
		availableLogLines = 5
	}

	// Show last N lines based on available space (fill to bottom)
	start := 0
	if len(m.logLines) > availableLogLines {
		start = len(m.logLines) - availableLogLines
	}

	for _, line := range m.logLines[start:] {
		// Truncate long lines to fit window width
		if len(line) > m.width-2 {
			line = line[:m.width-5] + "..."
		}
		lines = append(lines, logStyle.Render(line))
	}

	return "\n" + strings.Join(lines, "\n")
}

func (m Model) renderControls() string {
	logStatus := "Show"
	if m.showLogs {
		logStatus = "Hide"
	}

	var controls string
	if m.ripState == StateScanning || m.ripState == StateRipping || m.ripState == StateSelectingTitles {
		controls = fmt.Sprintf("[Q] Quit  [X/E] Cancel & Eject  [C] Clear  [T] Retry  [A] Scan  [L] %s Logs", logStatus)
	} else if m.currentEncode != nil {
		// Show encode-specific controls when actively encoding
		pauseText := "Pause"
		if m.encodePaused {
			pauseText = "Resume"
		}
		controls = fmt.Sprintf("[Q] Quit  [Space] %s  [S] Stop  [D] Delete  [C] Clear  [T] Retry  [A] Scan  [L] %s Logs", pauseText, logStatus)
	} else {
		controls = fmt.Sprintf("[Q] Quit  [C] Clear  [T] Retry  [A] Scan for Missing  [L] %s Logs", logStatus)
	}

	controlsStyle := lipgloss.NewStyle().Faint(true)
	return controlsStyle.Render(controls)
}
