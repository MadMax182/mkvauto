package app

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"
	"github.com/mmzim/mkvauto/internal/config"
	"github.com/mmzim/mkvauto/internal/disk"
	"github.com/mmzim/mkvauto/internal/encode"
	"github.com/mmzim/mkvauto/internal/makemkv"
	"github.com/mmzim/mkvauto/internal/notify"
	"github.com/mmzim/mkvauto/internal/ui"
)

type App struct {
	config           *config.Config
	queue            *encode.Queue
	makemkvClient    *makemkv.Client
	diskDetector     *disk.Detector
	notifier         *notify.DiscordWebhook
	workerControl    chan encode.WorkerControl
	titleSelectionCh chan []int
	cancelRipCh      chan struct{}
	scanRequestCh    chan struct{}
	program          *tea.Program
	logFile          *os.File
}

func New(cfg *config.Config) *App {
	// Create queue state directory
	homeDir, _ := os.UserHomeDir()
	stateDir := filepath.Join(homeDir, ".mkvauto")
	statePath := filepath.Join(stateDir, "queue.json")

	return &App{
		config:           cfg,
		queue:            encode.NewQueue(statePath),
		makemkvClient:    makemkv.NewClient(cfg.MakeMKV.BinaryPath),
		diskDetector:     disk.NewDetector(cfg.Drive.Path),
		notifier:         notify.NewDiscordWebhook(cfg.DiscordWebhook),
		workerControl:    make(chan encode.WorkerControl, 10),
		titleSelectionCh: make(chan []int, 1),
		cancelRipCh:      make(chan struct{}, 1),
		scanRequestCh:    make(chan struct{}, 1),
	}
}

func (a *App) Run() error {
	// Create lock file to prevent multiple instances
	homeDir, _ := os.UserHomeDir()
	lockPath := filepath.Join(homeDir, ".mkvauto", "mkvauto.lock")
	os.MkdirAll(filepath.Dir(lockPath), 0755)

	// Try to create lock file
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		if os.IsExist(err) {
			// Lock file exists, check if process is still running
			if isProcessRunning(lockPath) {
				return fmt.Errorf("another instance of mkvauto is already running (lock file exists: %s)", lockPath)
			}
			// Stale lock file, remove it and try again
			os.Remove(lockPath)
			lockFile, err = os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				return fmt.Errorf("failed to create lock file: %w", err)
			}
		} else {
			return fmt.Errorf("failed to create lock file: %w", err)
		}
	}
	defer func() {
		lockFile.Close()
		os.Remove(lockPath)
	}()

	// Write PID to lock file
	fmt.Fprintf(lockFile, "%d\n", os.Getpid())

	// Load queue state from disk
	if err := a.queue.LoadState(); err != nil {
		return fmt.Errorf("failed to load queue state: %w", err)
	}

	// Create log file (truncate existing)
	logPath := filepath.Join(homeDir, ".mkvauto", "mkvauto.log")
	os.MkdirAll(filepath.Dir(logPath), 0755)

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer logFile.Close()
	a.logFile = logFile

	// Write session start marker
	fmt.Fprintf(logFile, "=== Session started at %s ===\n", time.Now().Format(time.RFC3339))

	// Create context for goroutines
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start encoding worker
	progressCh := make(chan encode.ProgressUpdate, 10)
	logCh := make(chan string, 100)
	go a.startEncodingWorker(ctx, progressCh, logCh)

	// Start disk detector
	diskCh := a.diskDetector.Start(ctx)

	// Initialize TUI
	model := ui.NewModel(a.queue, a.workerControl, a.titleSelectionCh, a.config.OutputDir, a.cancelRipCh, a.scanRequestCh)
	a.program = tea.NewProgram(model, tea.WithAltScreen())

	// Start background goroutines
	go a.handleDisks(ctx, diskCh, a.program, logCh)
	go a.handleEncodeProgress(ctx, progressCh, a.program)
	go a.handleLogs(ctx, logCh, a.program)
	go a.handleScanRequests(ctx, logCh)

	// Run the TUI
	if _, err := a.program.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	return nil
}

func (a *App) startEncodingWorker(ctx context.Context, progressCh chan<- encode.ProgressUpdate, logCh chan<- string) {
	handbrake := encode.NewHandBrake(a.config)
	worker := encode.NewWorker(a.queue, handbrake, progressCh, a.workerControl, logCh)
	worker.Run(ctx)
}

func (a *App) handleDisks(ctx context.Context, diskCh <-chan disk.DetectedDisc, program *tea.Program, logCh chan<- string) {
	for {
		select {
		case <-ctx.Done():
			return
		case disc := <-diskCh:
			// Process disc in a goroutine (non-blocking)
			go a.processDisc(ctx, disc, program, logCh)
		}
	}
}

func (a *App) handleLogs(ctx context.Context, logCh <-chan string, program *tea.Program) {
	for {
		select {
		case <-ctx.Done():
			return
		case logLine := <-logCh:
			// Send to TUI
			program.Send(ui.LogMsg{Line: logLine})

			// Write to log file
			if a.logFile != nil {
				fmt.Fprintln(a.logFile, logLine)
			}
		}
	}
}

func (a *App) handleEncodeProgress(ctx context.Context, progressCh <-chan encode.ProgressUpdate, program *tea.Program) {
	for {
		select {
		case <-ctx.Done():
			return
		case update := <-progressCh:
			program.Send(ui.EncodeProgressMsg{
				ItemID:   update.ItemID,
				Progress: update.Progress,
			})

			// Check if encode just completed
			if update.Progress >= 100.0 {
				item := a.queue.GetCurrent()
				if item != nil {
					// Send Discord notification
					a.notifier.SendEncodeComplete(item.TitleName, item.DiscType.String())
					program.Send(ui.EncodeCompleteMsg{ItemID: item.ID})
				}
			}
		}
	}
}

func (a *App) handleScanRequests(ctx context.Context, logCh chan<- string) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-a.scanRequestCh:
			// Run scan in goroutine to avoid blocking
			go a.scanForMissingEncodes(logCh)
		}
	}
}

func formatDuration(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60
	return fmt.Sprintf("%d:%02d:%02d", hours, minutes, seconds)
}

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func (a *App) processDisc(ctx context.Context, disc disk.DetectedDisc, program *tea.Program, logCh chan<- string) {
	// Create cancellable context for this disc processing
	ripCtx, cancelRip := context.WithCancel(ctx)
	defer cancelRip()

	// Track if manually cancelled
	manuallyCancelled := false

	// Monitor for cancel requests
	go func() {
		select {
		case <-a.cancelRipCh:
			manuallyCancelled = true
			cancelRip()
			disk.Eject(disc.Device)
		case <-ripCtx.Done():
		}
	}()

	// Notify TUI
	program.Send(ui.DiskInsertedMsg{})

	// Create status channel for scan updates
	scanStatusCh := make(chan string, 10)
	go func() {
		for status := range scanStatusCh {
			// Prefix with "Scan: " to make it clear this is the initial scan
			program.Send(ui.StatusUpdateMsg{Status: "Scan: " + status})
		}
	}()

	// Scan disc with MakeMKV
	scanResult, err := a.makemkvClient.ScanDisc(ripCtx, disc.Device, scanStatusCh)
	close(scanStatusCh)

	if err != nil {
		// Check if context was cancelled (manual cancellation)
		if ripCtx.Err() != nil {
			return
		}

		program.Send(ui.ErrorMsg{Err: fmt.Errorf("scan failed: %w", err)})
		// Don't send Discord notification if manually cancelled
		if !manuallyCancelled {
			a.notifier.SendError("Disc Scan", err.Error())
		}
		return
	}

	// Update disc info
	disc.Name = disk.SanitizeFilename(scanResult.DiscName)
	disc.DiscType = disk.DetectDiscTypeFromInfo(scanResult.DiscType)

	program.Send(ui.ScanCompleteMsg{
		Info: ui.DiskInfo{
			Name:     scanResult.DiscName,
			DiscType: disc.DiscType.String(),
		},
	})

	// Select titles based on duration logic
	movieThreshold := time.Duration(a.config.Thresholds.MovieMinMinutes) * time.Minute
	episodeThreshold := time.Duration(a.config.Thresholds.EpisodeMinMinutes) * time.Minute
	selectedTitles := makemkv.SelectTitles(scanResult.Titles, movieThreshold, episodeThreshold)

	// If no titles matched, show manual selection UI
	if len(selectedTitles) == 0 {
		// Convert titles to UI format
		uiTitles := make([]ui.Title, len(scanResult.Titles))
		for i, t := range scanResult.Titles {
			uiTitles[i] = ui.Title{
				ID:       t.ID,
				Name:     t.Name,
				Duration: formatDuration(t.Duration),
				Size:     formatSize(t.Size),
				Selected: false,
			}
		}

		// Show title selection UI
		program.Send(ui.ShowTitleSelectionMsg{Titles: uiTitles})

		// Wait for user selection
		selectedIDs := <-a.titleSelectionCh

		if len(selectedIDs) == 0 {
			program.Send(ui.ErrorMsg{Err: fmt.Errorf("no titles selected")})
			disk.Eject(disc.Device)
			return
		}

		// Build selectedTitles from IDs
		selectedTitles = nil
		for _, id := range selectedIDs {
			for _, t := range scanResult.Titles {
				if t.ID == id {
					selectedTitles = append(selectedTitles, t)
					break
				}
			}
		}
	}

	// Create disc folder (no timestamp - will reuse folder for same disc)
	discFolder := filepath.Join(a.config.OutputDir, disc.Name)
	rawFolder := filepath.Join(discFolder, "raw")
	encodedFolder := filepath.Join(discFolder, "encoded")

	// Create directories (will reuse if already exists)
	if err := os.MkdirAll(rawFolder, 0755); err != nil {
		program.Send(ui.ErrorMsg{Err: fmt.Errorf("failed to create output directory: %w", err)})
		disk.Eject(disc.Device)
		return
	}
	if err := os.MkdirAll(encodedFolder, 0755); err != nil {
		program.Send(ui.ErrorMsg{Err: fmt.Errorf("failed to create output directory: %w", err)})
		disk.Eject(disc.Device)
		return
	}

	// Rip each selected title
	for i, title := range selectedTitles {
		// Notify that we're starting to rip this title
		program.Send(ui.StatusUpdateMsg{Status: fmt.Sprintf("Preparing to rip title %d of %d...", i+1, len(selectedTitles))})

		program.Send(ui.RipProgressMsg{
			Progress:     0,
			CurrentTitle: i + 1,
			TotalTitles:  len(selectedTitles),
		})

		// Note: We don't know the exact filename MakeMKV will create yet,
		// it uses disc name + _t## format, so we'll find it after ripping

		// Rip with progress updates
		ripProgressCh := make(chan float64, 10)
		go func() {
			for progress := range ripProgressCh {
				program.Send(ui.RipProgressMsg{
					Progress:     progress,
					CurrentTitle: i + 1,
					TotalTitles:  len(selectedTitles),
				})
			}
		}()

		// Create log channel that also sends status updates
		ripLogCh := make(chan string, 100)
		go func() {
			for line := range ripLogCh {
				// Check if it's a status line and send to TUI
				if strings.HasPrefix(line, "STATUS: ") {
					status := strings.TrimPrefix(line, "STATUS: ")
					// Prefix with "Rip: " to distinguish from scan phase
					program.Send(ui.StatusUpdateMsg{Status: "Rip: " + status})
				}
				// Also send to main log channel
				logCh <- line
			}
		}()

		err := a.makemkvClient.RipTitle(ripCtx, disc.Device, title.ID, rawFolder, ripProgressCh, ripLogCh)
		close(ripProgressCh)
		close(ripLogCh)

		if err != nil {
			program.Send(ui.ErrorMsg{Err: fmt.Errorf("rip failed: %w", err)})
			// Don't send Discord notification if manually cancelled
			if !manuallyCancelled {
				a.notifier.SendError("Disc Rip", err.Error())
			}
			continue
		}

		// Find the actual file that MakeMKV created (it uses its own naming scheme)
		actualRawPath, err := findNewestMKVFile(rawFolder)
		if err != nil {
			program.Send(ui.ErrorMsg{Err: fmt.Errorf("could not find ripped file: %w", err)})
			continue
		}

		// Use the actual filename for the encoded output
		actualFilename := filepath.Base(actualRawPath)
		actualEncodedPath := filepath.Join(encodedFolder, actualFilename)

		// Add to encoding queue with actual file paths
		queueItem := &encode.QueueItem{
			ID:         uuid.New().String(),
			SourcePath: actualRawPath,
			DestPath:   actualEncodedPath,
			DiscType:   disc.DiscType,
			DiscName:   scanResult.DiscName,
			TitleName:  title.Name,
			Status:     encode.StatusQueued,
			Progress:   0,
			CreatedAt:  time.Now(),
		}
		a.queue.Add(queueItem)
	}

	// Check if manually cancelled before sending completion
	if !manuallyCancelled && ripCtx.Err() == nil {
		// Send completion notification
		program.Send(ui.RipCompleteMsg{})
		a.notifier.SendRipComplete(scanResult.DiscName, len(selectedTitles), disc.DiscType.String())
	}

	// Eject disc
	disk.Eject(disc.Device)
}

// isProcessRunning checks if the process in the lock file is still running
func isProcessRunning(lockPath string) bool {
	data, err := ioutil.ReadFile(lockPath)
	if err != nil {
		return false
	}

	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return false
	}

	// Check if process exists by sending signal 0
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// Signal 0 doesn't actually send a signal, just checks if process exists
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// findNewestMKVFile finds the most recently modified MKV file in a directory
func findNewestMKVFile(dir string) (string, error) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return "", err
	}

	var newestFile string
	var newestTime time.Time

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		// Check if it's an MKV file
		if !strings.HasSuffix(strings.ToLower(file.Name()), ".mkv") {
			continue
		}

		fullPath := filepath.Join(dir, file.Name())
		if file.ModTime().After(newestTime) {
			newestTime = file.ModTime()
			newestFile = fullPath
		}
	}

	if newestFile == "" {
		return "", fmt.Errorf("no MKV files found in %s", dir)
	}

	return newestFile, nil
}

// scanForMissingEncodes scans the output directory for raw files that don't have corresponding encoded files
func (a *App) scanForMissingEncodes(logCh chan<- string) error {
	logCh <- "Scanning for raw files missing encoded versions..."

	// Read all directories in output directory
	dirs, err := ioutil.ReadDir(a.config.OutputDir)
	if err != nil {
		return fmt.Errorf("failed to read output directory: %w", err)
	}

	addedCount := 0

	for _, dir := range dirs {
		if !dir.IsDir() {
			continue
		}

		discFolder := filepath.Join(a.config.OutputDir, dir.Name())
		rawFolder := filepath.Join(discFolder, "raw")
		encodedFolder := filepath.Join(discFolder, "encoded")

		// Check if raw folder exists
		if _, err := os.Stat(rawFolder); os.IsNotExist(err) {
			continue
		}

		// Read all files in raw folder
		rawFiles, err := ioutil.ReadDir(rawFolder)
		if err != nil {
			continue
		}

		for _, rawFile := range rawFiles {
			if rawFile.IsDir() || !strings.HasSuffix(strings.ToLower(rawFile.Name()), ".mkv") {
				continue
			}

			sourcePath := filepath.Join(rawFolder, rawFile.Name())
			destPath := filepath.Join(encodedFolder, rawFile.Name())

			// Check if encoded version already exists
			if _, err := os.Stat(destPath); err == nil {
				continue // Encoded file exists, skip
			}

			// Check if already in queue
			if a.queue.HasSourcePath(sourcePath) {
				continue // Already in queue, skip
			}

			// Determine disc type based on file size
			discType := disk.DiscTypeDVD
			if rawFile.Size() > 8*1024*1024*1024 { // >8GB = BluRay
				discType = disk.DiscTypeBluRay
			}

			// Add to queue
			item := &encode.QueueItem{
				ID:         uuid.New().String(),
				SourcePath: sourcePath,
				DestPath:   destPath,
				DiscType:   discType,
				DiscName:   dir.Name(),
				TitleName:  rawFile.Name(),
				Status:     encode.StatusQueued,
			}

			if err := a.queue.Add(item); err != nil {
				logCh <- fmt.Sprintf("Failed to add %s to queue: %v", rawFile.Name(), err)
				continue
			}

			logCh <- fmt.Sprintf("Added to queue: %s", rawFile.Name())
			addedCount++
		}
	}

	if addedCount == 0 {
		logCh <- "No missing encodes found"
	} else {
		logCh <- fmt.Sprintf("Added %d item(s) to encoding queue", addedCount)
	}

	return nil
}
