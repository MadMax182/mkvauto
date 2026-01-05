package encode

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/creack/pty"
	"github.com/mmzim/mkvauto/internal/config"
	"github.com/mmzim/mkvauto/internal/disk"
)

type HandBrake struct {
	config  *config.Config
	cmd     *exec.Cmd
	paused  bool
	pauseMu sync.Mutex
}

func NewHandBrake(cfg *config.Config) *HandBrake {
	return &HandBrake{
		config: cfg,
	}
}

// Encode encodes a video file using HandBrake
func (hb *HandBrake) Encode(ctx context.Context, item *QueueItem, progressCh chan<- float64, logCh chan<- string) error {
	args := hb.buildArgs(item)

	hb.pauseMu.Lock()
	hb.cmd = exec.CommandContext(ctx, hb.config.HandBrake.BinaryPath, args...)
	hb.pauseMu.Unlock()

	// Start with a PTY to get unbuffered output
	ptmx, err := pty.Start(hb.cmd)
	if err != nil {
		return fmt.Errorf("failed to start HandBrakeCLI with PTY: %w", err)
	}
	defer ptmx.Close()

	// Parse progress from PTY output
	progressRegex := regexp.MustCompile(`(?:Encoding:|Progress:).*?(\d+\.\d+)\s*%`)

	// Read from PTY
	done := make(chan struct{})
	go func() {
		defer close(done)

		buf := make([]byte, 1)
		var currentLine strings.Builder

		for {
			n, err := ptmx.Read(buf)
			if err != nil {
				if err != io.EOF {
					if logCh != nil {
						select {
						case logCh <- fmt.Sprintf("[PTY-ERROR] %v", err):
						default:
						}
					}
				}
				return
			}
			if n == 0 {
				continue
			}

			b := buf[0]

			// Check for line delimiters
			if b == '\n' || b == '\r' {
				line := currentLine.String()
				if line != "" {
					// Debug: log if we see "Encoding" anywhere
					if logCh != nil && (strings.Contains(line, "Encoding") || strings.Contains(line, "%")) {
						select {
						case logCh <- fmt.Sprintf("[HB-RAW] %s", line):
						case <-ctx.Done():
							return
						default:
						}
					}

					// Send to log channel (non-progress lines only)
					if logCh != nil && !strings.HasPrefix(line, "Encoding:") && !strings.HasPrefix(line, "Progress:") {
						select {
						case logCh <- line:
						case <-ctx.Done():
							return
						default:
						}
					}

					// Look for progress updates
					matches := progressRegex.FindStringSubmatch(line)
					if len(matches) > 1 {
						if percentage, err := strconv.ParseFloat(matches[1], 64); err == nil {
							if logCh != nil {
								select {
								case logCh <- fmt.Sprintf("[PROGRESS-PARSED] %.2f%%", percentage):
								case <-ctx.Done():
									return
								default:
								}
							}
							select {
							case progressCh <- percentage:
							case <-ctx.Done():
								return
							default:
							}
						}
					}
				}
				currentLine.Reset()
			} else {
				currentLine.WriteByte(b)
			}
		}
	}()

	// Wait for command to complete
	err = hb.cmd.Wait()
	<-done // Wait for reader to finish

	if err != nil {
		return fmt.Errorf("HandBrakeCLI failed: %w", err)
	}

	// Send 100% when complete
	select {
	case progressCh <- 100.0:
	case <-ctx.Done():
	default:
	}

	return nil
}

// buildArgs constructs HandBrake command-line arguments based on profile
func (hb *HandBrake) buildArgs(item *QueueItem) []string {
	var profile config.HandBrakeProfile

	// Select profile based on disc type
	if item.DiscType == disk.DiscTypeBluRay {
		profile = hb.config.HandBrake.BluRay
	} else {
		profile = hb.config.HandBrake.DVD
	}

	args := []string{
		"-i", item.SourcePath,
		"-o", item.DestPath,
	}

	// Use preset file if specified
	if profile.PresetFile != "" {
		// Build full path from presets directory
		presetPath := profile.PresetFile
		if hb.config.HandBrake.PresetsDir != "" {
			presetPath = hb.config.HandBrake.PresetsDir + "/" + profile.PresetFile
		}

		args = append(args, "--preset-import-file", presetPath)

		// If preset_name is specified, use it. Otherwise HandBrake will use the first preset in the file
		if profile.PresetName != "" {
			args = append(args, "--preset", profile.PresetName)
		}
	}

	// Override audio languages if specified
	if len(profile.AudioLanguages) > 0 {
		langs := strings.Join(profile.AudioLanguages, ",")
		args = append(args, "--audio-lang-list", langs)
		// Also select first audio track (prevents selecting all)
		args = append(args, "--first-audio")
	}

	// Override subtitle languages if specified
	if len(profile.SubtitleLanguages) > 0 {
		langs := strings.Join(profile.SubtitleLanguages, ",")
		args = append(args, "--subtitle-lang-list", langs)
	}

	// Set thread count if specified (0 = auto)
	// For SVT-AV1 and other encoders, pass threads as encoder options
	if hb.config.HandBrake.Threads > 0 {
		args = append(args, "--encopts", fmt.Sprintf("threads=%d", hb.config.HandBrake.Threads))
	}

	return args
}

// Pause pauses the HandBrake process
func (hb *HandBrake) Pause() error {
	hb.pauseMu.Lock()
	defer hb.pauseMu.Unlock()

	if hb.cmd != nil && hb.cmd.Process != nil && !hb.paused {
		hb.paused = true
		return hb.cmd.Process.Signal(syscall.SIGSTOP)
	}

	return nil
}

// Resume resumes the HandBrake process
func (hb *HandBrake) Resume() error {
	hb.pauseMu.Lock()
	defer hb.pauseMu.Unlock()

	if hb.cmd != nil && hb.cmd.Process != nil && hb.paused {
		hb.paused = false
		return hb.cmd.Process.Signal(syscall.SIGCONT)
	}

	return nil
}

// Cancel cancels the encoding process
func (hb *HandBrake) Cancel() error {
	hb.pauseMu.Lock()
	defer hb.pauseMu.Unlock()

	if hb.cmd != nil && hb.cmd.Process != nil {
		return hb.cmd.Process.Kill()
	}

	return nil
}
