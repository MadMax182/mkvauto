package makemkv

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

type Client struct {
	binaryPath string
}

func NewClient(binaryPath string) *Client {
	return &Client{
		binaryPath: binaryPath,
	}
}

// ScanDisc scans the disc and returns information about titles
// statusCh receives status updates during the scan
func (c *Client) ScanDisc(ctx context.Context, devicePath string, statusCh chan<- string) (*ScanResult, error) {
	cmd := exec.CommandContext(ctx, c.binaryPath, "-r", "--progress=-stdout", "info", "disc:0")

	// Get stdout pipe to read status messages
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	var outputLines []string
	var outputMu sync.Mutex

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start makemkvcon: %w", err)
	}

	// Read and parse output
	scanner := bufio.NewScanner(stdout)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

		// Store for parsing
		outputMu.Lock()
		outputLines = append(outputLines, line)
		outputMu.Unlock()

		// Check for status updates
		if status, ok := ParseStatusMessage(line); ok && statusCh != nil {
			select {
			case statusCh <- status:
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
		}
	}

	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("makemkvcon info failed: %w", err)
	}

	outputMu.Lock()
	fullOutput := strings.Join(outputLines, "\n")
	outputMu.Unlock()

	return ParseInfo(fullOutput)
}

// RipTitle rips a single title to the output directory
// Sends progress updates to the progressCh channel
func (c *Client) RipTitle(ctx context.Context, devicePath string, titleID int, outputDir string, progressCh chan<- float64, logCh chan<- string) error {
	// makemkvcon -r --progress=-stdout mkv disc:0 titleID outputDir
	cmd := exec.CommandContext(ctx, c.binaryPath, "-r", "--progress=-stdout", "mkv", "disc:0", fmt.Sprintf("%d", titleID), outputDir)

	// Close stdin to prevent any prompts from blocking
	cmd.Stdin = nil

	// Get stdout pipe to read progress
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start makemkvcon: %w", err)
	}

	// Read progress from stdout
	go func() {
		scanner := bufio.NewScanner(stdout)
		// Increase buffer size for long lines
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)

		for scanner.Scan() {
			line := scanner.Text()

			// Send to log channel
			if logCh != nil {
				select {
				case logCh <- line:
				case <-ctx.Done():
					return
				default:
				}
			}

			// Check for status updates
			if status, ok := ParseStatusMessage(line); ok && logCh != nil {
				select {
				case logCh <- "STATUS: " + status:
				case <-ctx.Done():
					return
				default:
				}
			}

			// Check for progress updates
			if current, total, max, ok := ParseProgress(line); ok {
				percentage := CalculatePercentage(current, total, max)
				select {
				case progressCh <- percentage:
				case <-ctx.Done():
					return
				default:
					// Don't block if channel is not being read
				}
			}
		}
	}()

	// Also read stderr to avoid blocking
	go func() {
		scanner := bufio.NewScanner(stderr)
		// Increase buffer size for long lines
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)

		for scanner.Scan() {
			line := scanner.Text()
			// Send stderr to log channel
			if logCh != nil {
				select {
				case logCh <- line:
				case <-ctx.Done():
					return
				default:
				}
			}
		}
	}()

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("makemkvcon mkv failed: %w", err)
	}

	// Send 100% when complete
	select {
	case progressCh <- 100.0:
	case <-ctx.Done():
	default:
	}

	return nil
}

// CheckVersion checks if makemkvcon is available
func (c *Client) CheckVersion() (string, error) {
	cmd := exec.Command(c.binaryPath, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("makemkvcon not found or not working: %w", err)
	}

	version := strings.TrimSpace(string(output))
	return version, nil
}
