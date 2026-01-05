package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/viper"
)

type Config struct {
	OutputDir       string       `mapstructure:"output_dir"`
	DiscordWebhook  string       `mapstructure:"discord_webhook"`
	Drive           DriveConfig  `mapstructure:"drive"`
	Thresholds      Thresholds   `mapstructure:"thresholds"`
	MakeMKV         MakeMKVConfig `mapstructure:"makemkv"`
	HandBrake       HandBrakeConfig `mapstructure:"handbrake"`
}

type DriveConfig struct {
	Path string `mapstructure:"path"`
}

type Thresholds struct {
	MovieMinMinutes   int `mapstructure:"movie_min_minutes"`
	EpisodeMinMinutes int `mapstructure:"episode_min_minutes"`
}

type MakeMKVConfig struct {
	BinaryPath string `mapstructure:"binary_path"`
}

type HandBrakeConfig struct {
	BinaryPath  string              `mapstructure:"binary_path"`
	PresetsDir  string              `mapstructure:"presets_dir"`
	Threads     int                 `mapstructure:"threads"` // Number of threads (0 = auto)
	BluRay      HandBrakeProfile    `mapstructure:"bluray"`
	DVD         HandBrakeProfile    `mapstructure:"dvd"`
}

type HandBrakeProfile struct {
	PresetFile        string   `mapstructure:"preset_file"`        // Filename in presets_dir
	PresetName        string   `mapstructure:"preset_name"`        // Name of preset within the file
	AudioLanguages    []string `mapstructure:"audio_languages"`    // Audio languages to include (e.g., ["eng"])
	SubtitleLanguages []string `mapstructure:"subtitle_languages"` // Subtitle languages to include (e.g., ["eng"])
}

// Load reads the configuration from the config file
func Load(configPath string) (*Config, error) {
	v := viper.New()

	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		// Look for config in home directory or current directory
		home, err := os.UserHomeDir()
		if err == nil {
			v.AddConfigPath(filepath.Join(home, ".config", "mkvauto"))
		}
		v.AddConfigPath(".")
		v.SetConfigName("config")
		v.SetConfigType("yaml")
	}

	// Set defaults
	v.SetDefault("drive.path", "/dev/sr0")
	v.SetDefault("thresholds.movie_min_minutes", 60)
	v.SetDefault("thresholds.episode_min_minutes", 18)
	v.SetDefault("makemkv.binary_path", "makemkvcon")
	v.SetDefault("handbrake.binary_path", "HandBrakeCLI")

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(cfg.OutputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	return &cfg, nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.OutputDir == "" {
		return fmt.Errorf("output_dir is required")
	}
	if c.DiscordWebhook == "" {
		return fmt.Errorf("discord_webhook is required")
	}
	if c.Drive.Path == "" {
		return fmt.Errorf("drive.path is required")
	}

	// Check if MakeMKV binary exists
	if _, err := exec.LookPath(c.MakeMKV.BinaryPath); err != nil {
		return fmt.Errorf("makemkv binary not found: %s", c.MakeMKV.BinaryPath)
	}

	// Check if HandBrake binary exists
	if _, err := exec.LookPath(c.HandBrake.BinaryPath); err != nil {
		return fmt.Errorf("handbrake binary not found: %s", c.HandBrake.BinaryPath)
	}

	return nil
}
