# mkvauto

Automated MakeMKV disc ripping and HandBrake encoding TUI written in Go.

## Features

- Automatic disc detection and ripping with MakeMKV
- Queued encoding with HandBrake (SVT-AV1)
- Real-time progress tracking with ETA
- Manual title selection
- Discord notifications
- Pause/resume/cancel support
- Queue persistence

## Usage

### Basic Operation

Start the TUI:
```bash
./mkvauto
```

Insert a disc and the application will automatically:
1. Scan the disc
2. Allow you to select titles to rip
3. Rip selected titles to the output directory
4. Queue them for encoding
5. Encode using your configured HandBrake preset

### Manually Add Files to Encoding Queue

You can manually add existing video files to the encoding queue without ripping:

```bash
# Add a single file (auto-detect disc type based on file size)
./mkvauto --add "/path/to/video.mkv" --output "/path/to/output.mkv"

# Specify disc type explicitly (bluray or dvd)
./mkvauto --add "/path/to/video.mkv" --type bluray --output "/path/to/output.mkv"

# Auto-detection: files >8GB = BluRay, <=8GB = DVD
./mkvauto --add "/path/to/video.mkv" --type auto --output "/path/to/output.mkv"
```

**Options:**
- `--add` - Path to the video file to encode
- `--type` - Disc type: `bluray`, `dvd`, or `auto` (default: auto)
- `--output` - Path for the encoded output file

**Example:**
```bash
./mkvauto --add "~/Videos/movie.mkv" --type bluray --output "~/Videos/encoded/movie.mkv"
```

The file will be added to the queue and the TUI will start. You can add multiple files by running the command multiple times.

### Scan for Missing Encodes

Press **A** in the TUI to automatically scan your output directory for raw MKV files that don't have corresponding encoded versions. The scanner will:

1. Search all subdirectories in your output folder
2. Look for folders with a `raw/` subfolder containing MKV files
3. Check if each raw file has a matching encoded file in the `encoded/` subfolder
4. Auto-detect disc type based on file size (>8GB = BluRay, ≤8GB = DVD)
5. Add missing files to the encoding queue

This is useful for:
- Re-encoding previously ripped discs with new settings
- Adding files after clearing completed items
- Recovering from interrupted encoding sessions

### Controls

#### During Ripping
- **X/E** - Cancel ripping and eject disc
- **P** - Pause ripping
- **R** - Resume ripping

#### During Encoding
- **Space** - Pause/Resume current encode
- **S** - Stop current encode (keeps in queue as failed, can retry)
- **D** - Delete current encode (stop and remove from queue)

#### Queue Management
- **C** - Clear completed and failed items from queue
- **T** - Retry all failed items
- **A** - Scan for raw files missing encoded versions (auto-add to queue)

#### General
- **L** - Toggle log view
- **Q** - Quit application

## Configuration

Create a `config.yaml` file. See `config.example.yaml` for reference.

### Thread Control

Limit HandBrake encoding threads in your config:

```yaml
handbrake:
  binary_path: "HandBrakeCLI"
  presets_dir: "/path/to/presets"
  threads: 8  # Set to specific number, or 0 for auto
```

## Requirements

- MakeMKV (`makemkvcon`)
- HandBrake CLI (`HandBrakeCLI`)
- Go 1.21+ (for building)

## Installation

### Automated Installation (Recommended)

The install script will:
- Install all dependencies (makemkv, handbrake-cli, go)
- Set up MakeMKV license key
- Configure user permissions
- Create configuration files
- Build and install mkvauto
- Optionally set up a private pacman repository

```bash
git clone https://github.com/yourusername/mkvauto.git
cd mkvauto
./install.sh
```

The installer will prompt you for:
- MakeMKV beta license key (get from: https://forum.makemkv.com/forum/viewtopic.php?t=1053)
- Discord webhook URL (optional)
- Output directory for encoded videos
- Installation method (system-wide, private repo, or user-only)

### Manual Installation

```bash
# Install dependencies
yay -S makemkv handbrake-cli go

# Build
go build -o mkvauto ./cmd/mkvauto

# Install
sudo install -Dm755 mkvauto /usr/local/bin/mkvauto

# Create config
mkdir -p ~/.config/mkvauto
cp config.example.yaml ~/.config/mkvauto/config.yaml
nano ~/.config/mkvauto/config.yaml
```
