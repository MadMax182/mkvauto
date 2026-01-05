package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mmzim/mkvauto/internal/app"
	"github.com/mmzim/mkvauto/internal/config"
	"github.com/mmzim/mkvauto/internal/disk"
	"github.com/mmzim/mkvauto/internal/encode"
)

func main() {
	// Parse command-line flags
	addFile := flag.String("add", "", "Add a file to the encoding queue (path to MKV file)")
	addDiscType := flag.String("type", "auto", "Disc type for added file: bluray, dvd, or auto (default: auto)")
	addOutput := flag.String("output", "", "Output path for encoded file (default: same directory with _encoded suffix)")
	flag.Parse()

	// Load configuration
	configPath := os.Getenv("MKVAUTO_CONFIG")
	if configPath == "" {
		// Check default locations
		homeDir, err := os.UserHomeDir()
		if err == nil {
			configPath = homeDir + "/.config/mkvauto/config.yaml"
		}

		// Check if file exists
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			configPath = "./config.yaml"
		}
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		fmt.Fprintf(os.Stderr, "Please create a config file at %s\n", configPath)
		fmt.Fprintf(os.Stderr, "See config.example.yaml for an example.\n")
		os.Exit(1)
	}

	// Handle --add flag
	if *addFile != "" {
		if err := addFileToQueue(cfg, *addFile, *addDiscType, *addOutput); err != nil {
			fmt.Fprintf(os.Stderr, "Error adding file to queue: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Successfully added %s to encoding queue\n", *addFile)
		return
	}

	// Create and run application
	application := app.New(cfg)
	if err := application.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Application error: %v\n", err)
		os.Exit(1)
	}
}

func addFileToQueue(cfg *config.Config, sourcePath, discTypeStr, outputPath string) error {
	// Validate source file exists
	absSourcePath, err := filepath.Abs(sourcePath)
	if err != nil {
		return fmt.Errorf("invalid source path: %w", err)
	}

	if _, err := os.Stat(absSourcePath); os.IsNotExist(err) {
		return fmt.Errorf("source file does not exist: %s", absSourcePath)
	}

	// Determine disc type
	var discType disk.DiscType
	switch strings.ToLower(discTypeStr) {
	case "bluray", "blu-ray", "br":
		discType = disk.DiscTypeBluRay
	case "dvd":
		discType = disk.DiscTypeDVD
	case "auto":
		// Auto-detect based on file size (rough heuristic: >8GB = BluRay)
		info, err := os.Stat(absSourcePath)
		if err != nil {
			return fmt.Errorf("failed to stat file: %w", err)
		}
		if info.Size() > 8*1024*1024*1024 {
			discType = disk.DiscTypeBluRay
		} else {
			discType = disk.DiscTypeDVD
		}
	default:
		return fmt.Errorf("invalid disc type: %s (use bluray, dvd, or auto)", discTypeStr)
	}

	// Determine output path
	var absOutputPath string
	if outputPath != "" {
		absOutputPath, err = filepath.Abs(outputPath)
		if err != nil {
			return fmt.Errorf("invalid output path: %w", err)
		}
	} else {
		// Default: same directory, add _encoded suffix
		dir := filepath.Dir(absSourcePath)
		base := filepath.Base(absSourcePath)
		ext := filepath.Ext(base)
		nameWithoutExt := strings.TrimSuffix(base, ext)
		absOutputPath = filepath.Join(dir, nameWithoutExt+"_encoded"+ext)
	}

	// Create queue item
	item := &encode.QueueItem{
		ID:         uuid.New().String(),
		SourcePath: absSourcePath,
		DestPath:   absOutputPath,
		DiscType:   discType,
		DiscName:   "Manual",
		TitleName:  filepath.Base(absSourcePath),
		Status:     encode.StatusQueued,
		Progress:   0,
		CreatedAt:  time.Now(),
	}

	// Load queue and add item
	homeDir, _ := os.UserHomeDir()
	stateDir := filepath.Join(homeDir, ".mkvauto")
	statePath := filepath.Join(stateDir, "queue.json")

	queue := encode.NewQueue(statePath)
	if err := queue.LoadState(); err != nil {
		// Ignore error if queue file doesn't exist yet
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to load queue: %w", err)
		}
	}

	queue.Add(item)

	fmt.Printf("Added to queue:\n")
	fmt.Printf("  Source: %s\n", absSourcePath)
	fmt.Printf("  Output: %s\n", absOutputPath)
	fmt.Printf("  Type: %s\n", discType.String())

	return nil
}
