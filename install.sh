#!/bin/bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Banner
echo -e "${BLUE}"
cat << "EOF"
┌─────────────────────────────────────┐
│   MakeMKV Auto-Ripper Installer    │
│            mkvauto v1.0             │
└─────────────────────────────────────┘
EOF
echo -e "${NC}"

# Function to print colored messages
info() { echo -e "${BLUE}[INFO]${NC} $1"; }
success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

# Check if running as root
if [ "$EUID" -eq 0 ]; then
   error "Please do not run this script as root"
fi

# Detect AUR helper
info "Detecting AUR helper..."
if command -v yay &> /dev/null; then
    AUR_HELPER="yay"
elif command -v paru &> /dev/null; then
    AUR_HELPER="paru"
else
    error "No AUR helper found. Please install yay or paru first:
    git clone https://aur.archlinux.org/yay.git
    cd yay
    makepkg -si"
fi
success "Found AUR helper: $AUR_HELPER"

# ============================================
# STEP 1: Install Dependencies
# ============================================
info "Installing dependencies..."

# System packages
DEPS=(
    "go"                    # For building mkvauto
    "handbrake-cli"        # For encoding
    "git"                  # For version control
    "base-devel"          # For building packages
)

info "Installing system packages..."
sudo pacman -S --needed --noconfirm "${DEPS[@]}"

# AUR packages
AUR_DEPS=(
    "makemkv"             # MakeMKV (includes makemkvcon CLI)
)

info "Installing AUR packages (this may take a while)..."
$AUR_HELPER -S --needed --noconfirm "${AUR_DEPS[@]}"

success "All dependencies installed"

# ============================================
# STEP 2: Setup MakeMKV License Key
# ============================================
echo ""
echo -e "${YELLOW}════════════════════════════════════════════════════════════${NC}"
echo -e "${YELLOW}  MakeMKV License Key Setup${NC}"
echo -e "${YELLOW}════════════════════════════════════════════════════════════${NC}"
echo ""
info "MakeMKV requires a license key (free while in beta)"
info "Get your key from: https://forum.makemkv.com/forum/viewtopic.php?t=1053"
echo ""
read -p "Paste your MakeMKV beta key here: " MAKEMKV_KEY

if [ -z "$MAKEMKV_KEY" ]; then
    warn "No key provided. You'll need to enter it manually later in MakeMKV GUI"
else
    # Create MakeMKV settings directory
    mkdir -p ~/.MakeMKV

    # Create or update settings.conf
    SETTINGS_FILE="$HOME/.MakeMKV/settings.conf"

    if [ -f "$SETTINGS_FILE" ]; then
        # Remove old key if exists
        sed -i '/app_Key/d' "$SETTINGS_FILE"
    fi

    # Add new key
    echo "app_Key = \"$MAKEMKV_KEY\"" >> "$SETTINGS_FILE"
    success "MakeMKV license key configured"
fi

# ============================================
# STEP 3: Setup User Permissions
# ============================================
info "Setting up optical drive permissions..."

# Load sg module for MakeMKV SCSI access
if ! lsmod | grep -q "^sg "; then
    info "Loading sg kernel module..."
    sudo modprobe sg
    success "Loaded sg kernel module"
fi

# Make sg module load on boot
if [ ! -f /etc/modules-load.d/sg.conf ]; then
    info "Configuring sg module to load on boot..."
    echo "sg" | sudo tee /etc/modules-load.d/sg.conf > /dev/null
    success "sg module will load automatically on boot"
else
    success "sg module already configured to load on boot"
fi

# Add user to optical group for disc access
NEEDS_RESTART=false
if ! groups $USER | grep -q '\boptical\b'; then
    sudo usermod -aG optical $USER
    success "Added $USER to 'optical' group"
    NEEDS_RESTART=true
else
    success "User already in 'optical' group"
fi

# Add user to video group for hardware acceleration
if ! groups $USER | grep -q '\bvideo\b'; then
    sudo usermod -aG video $USER
    success "Added $USER to 'video' group"
fi

# ============================================
# STEP 4: Create Configuration Files
# ============================================
info "Creating configuration directories..."

CONFIG_DIR="$HOME/.config/mkvauto"
PRESETS_DIR="$CONFIG_DIR/presets"
STATE_DIR="$HOME/.mkvauto"

mkdir -p "$CONFIG_DIR"
mkdir -p "$PRESETS_DIR"
mkdir -p "$STATE_DIR"

success "Created directories:
  - $CONFIG_DIR
  - $PRESETS_DIR
  - $STATE_DIR"

# ============================================
# STEP 5: Setup Discord Webhook (Optional)
# ============================================
echo ""
echo -e "${YELLOW}════════════════════════════════════════════════════════════${NC}"
echo -e "${YELLOW}  Discord Webhook Setup (Optional)${NC}"
echo -e "${YELLOW}════════════════════════════════════════════════════════════${NC}"
echo ""
info "mkvauto can send notifications to Discord when tasks complete"
read -p "Do you want to configure Discord notifications? (y/N): " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo ""
    info "To create a Discord webhook:"
    info "1. Go to your Discord server settings"
    info "2. Go to Integrations > Webhooks"
    info "3. Click 'New Webhook'"
    info "4. Copy the webhook URL"
    echo ""
    read -p "Paste your Discord webhook URL: " DISCORD_WEBHOOK
else
    DISCORD_WEBHOOK="https://discord.com/api/webhooks/YOUR_WEBHOOK_URL"
    warn "Skipping Discord setup. You can add it later in config.yaml"
fi

# ============================================
# STEP 6: Create Config File
# ============================================
info "Creating configuration file..."

read -p "Where should encoded videos be saved? [$HOME/Videos/mkvauto]: " OUTPUT_DIR
OUTPUT_DIR=${OUTPUT_DIR:-$HOME/Videos/mkvauto}

# Create output directory
mkdir -p "$OUTPUT_DIR"

# Detect optical drive
DRIVE_PATH="/dev/sr0"
if [ ! -e "/dev/sr0" ] && [ -e "/dev/sr1" ]; then
    DRIVE_PATH="/dev/sr1"
fi

# Create config.yaml
cat > "$CONFIG_DIR/config.yaml" << EOF
output_dir: "$OUTPUT_DIR"

discord_webhook: "$DISCORD_WEBHOOK"

drive:
  path: "$DRIVE_PATH"

thresholds:
  movie_min_minutes: 60
  episode_min_minutes: 18

makemkv:
  binary_path: "makemkvcon"

handbrake:
  binary_path: "HandBrakeCLI"
  presets_dir: "$PRESETS_DIR"
  threads: 0  # 0 = auto, or set to specific number

  bluray:
    preset_file: "bluray.json"
    audio_languages: ["eng"]
    subtitle_languages: ["eng"]

  dvd:
    preset_file: "dvd.json"
    audio_languages: ["eng"]
    subtitle_languages: ["eng"]
EOF

success "Configuration file created at: $CONFIG_DIR/config.yaml"

# ============================================
# STEP 7: Setup HandBrake Presets
# ============================================
info "Setting up HandBrake presets..."

echo ""
warn "You need to create HandBrake preset files"
info "To create presets:"
info "1. Open HandBrake GUI: handbrake"
info "2. Configure your encoding settings (codec, quality, etc.)"
info "3. Click 'Presets' > 'Save New Preset'"
info "4. Export as JSON: 'Presets' > 'Export'"
info "5. Save to: $PRESETS_DIR/bluray.json and $PRESETS_DIR/dvd.json"
echo ""

read -p "Do you want to create basic preset files now? (Y/n): " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Nn]$ ]]; then
    # Create basic BluRay preset
    cat > "$PRESETS_DIR/bluray.json" << 'EOF'
{
  "PresetList": [
    {
      "PresetName": "1080p",
      "Folder": false,
      "Type": 1,
      "Default": false,
      "Audio": {
        "AudioList": [
          {
            "Bitrate": 160,
            "Encoder": "av_aac",
            "Mixdown": "stereo",
            "Track": 0
          },
          {
            "Bitrate": 640,
            "Encoder": "ac3",
            "Mixdown": "5point1",
            "Track": 0
          }
        ]
      },
      "Video": {
        "Encoder": "svt_av1_10bit",
        "Quality": 24.0,
        "Preset": "4"
      }
    }
  ]
}
EOF

    # Create basic DVD preset (same as BluRay for now)
    cp "$PRESETS_DIR/bluray.json" "$PRESETS_DIR/dvd.json"

    success "Created basic preset files (you can customize these later)"
fi

# ============================================
# STEP 8: Build mkvauto
# ============================================
info "Building mkvauto..."

INSTALL_DIR="$(dirname "$(readlink -f "$0")")"
cd "$INSTALL_DIR"

# Build the binary
go build -o mkvauto ./cmd/mkvauto

success "mkvauto built successfully"

# ============================================
# STEP 9: Install mkvauto
# ============================================
echo ""
echo -e "${YELLOW}════════════════════════════════════════════════════════════${NC}"
echo -e "${YELLOW}  Installation Method${NC}"
echo -e "${YELLOW}════════════════════════════════════════════════════════════${NC}"
echo ""
echo "Choose installation method:"
echo "  1) Install to /usr/local/bin (system-wide, simple)"
echo "  2) Create private pacman repository (recommended)"
echo "  3) Install to ~/.local/bin (user only)"
echo ""
read -p "Select option [1-3]: " INSTALL_METHOD

case $INSTALL_METHOD in
    1)
        info "Installing to /usr/local/bin..."
        sudo install -Dm755 mkvauto /usr/local/bin/mkvauto
        success "mkvauto installed to /usr/local/bin/mkvauto"
        ;;
    2)
        info "Creating private pacman repository..."

        # Create PKGBUILD if not exists
        if [ ! -f "PKGBUILD" ]; then
            cat > PKGBUILD << 'PKGBUILD_EOF'
# Maintainer: Your Name <email@example.com>
pkgname=mkvauto
pkgver=1.0.0
pkgrel=1
pkgdesc="Automated MakeMKV disc ripping and HandBrake encoding TUI"
arch=('x86_64')
url="https://github.com/mmzim/mkvauto"
license=('MIT')
depends=('makemkv' 'handbrake-cli')
makedepends=('go')

build() {
  cd "$startdir"
  export CGO_CPPFLAGS="${CPPFLAGS}"
  export CGO_CFLAGS="${CFLAGS}"
  export CGO_CXXFLAGS="${CXXFLAGS}"
  export CGO_LDFLAGS="${LDFLAGS}"
  export GOFLAGS="-buildmode=pie -trimpath -modcacherw"
  go build -o mkvauto ./cmd/mkvauto
}

package() {
  cd "$startdir"
  install -Dm755 mkvauto "$pkgdir/usr/bin/mkvauto"
  install -Dm644 config.example.yaml "$pkgdir/usr/share/doc/$pkgname/config.example.yaml"
  install -Dm644 README.md "$pkgdir/usr/share/doc/$pkgname/README.md"
}
PKGBUILD_EOF
            success "Created PKGBUILD"
        fi

        # Build package
        makepkg -sf

        # Setup private repo
        REPO_NAME="my-arch-repo"
        REPO_DIR="$HOME/.local/share/$REPO_NAME"
        mkdir -p "$REPO_DIR"

        # Copy package to repo
        cp mkvauto-*.pkg.tar.zst "$REPO_DIR/"

        # Create repo database
        cd "$REPO_DIR"
        repo-add "$REPO_NAME.db.tar.gz" *.pkg.tar.zst
        cd "$INSTALL_DIR"

        # Add to pacman.conf if not already there
        if ! sudo grep -q "\[$REPO_NAME\]" /etc/pacman.conf; then
            info "Adding repository to /etc/pacman.conf..."
            sudo tee -a /etc/pacman.conf > /dev/null <<EOF

[$REPO_NAME]
SigLevel = Optional TrustAll
Server = file://$REPO_DIR
EOF
            success "Repository added to pacman.conf"
        fi

        # Install via pacman
        sudo pacman -Sy
        sudo pacman -S --noconfirm mkvauto

        success "mkvauto installed via pacman"
        info "Update with: sudo pacman -Syu"
        ;;
    3)
        info "Installing to ~/.local/bin..."
        mkdir -p ~/.local/bin
        install -Dm755 mkvauto ~/.local/bin/mkvauto

        # Add to PATH if not already there
        if [[ ":$PATH:" != *":$HOME/.local/bin:"* ]]; then
            echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc
            warn "Added ~/.local/bin to PATH in ~/.bashrc"
            warn "Run 'source ~/.bashrc' or restart your terminal"
        fi

        success "mkvauto installed to ~/.local/bin/mkvauto"
        ;;
    *)
        error "Invalid option"
        ;;
esac

# ============================================
# STEP 10: Create Update Script
# ============================================
if [ "$INSTALL_METHOD" == "2" ]; then
    info "Creating update script..."
    cat > update-mkvauto.sh << 'UPDATE_EOF'
#!/bin/bash
set -e

cd "$(dirname "$(readlink -f "$0")")"

echo "Building mkvauto..."
makepkg -sf --clean

REPO_NAME="my-arch-repo"
REPO_DIR="$HOME/.local/share/$REPO_NAME"

echo "Updating repository..."
rm -f "$REPO_DIR"/mkvauto-*.pkg.tar.zst
cp mkvauto-*.pkg.tar.zst "$REPO_DIR/"

cd "$REPO_DIR"
repo-add -n -R "$REPO_NAME.db.tar.gz" *.pkg.tar.zst

echo "Upgrading package..."
sudo pacman -Sy
sudo pacman -S mkvauto

echo "Update complete!"
UPDATE_EOF
    chmod +x update-mkvauto.sh
    success "Created update script: ./update-mkvauto.sh"
fi

# ============================================
# Final Steps
# ============================================
echo ""
echo -e "${GREEN}════════════════════════════════════════════════════════════${NC}"
echo -e "${GREEN}  Installation Complete!${NC}"
echo -e "${GREEN}════════════════════════════════════════════════════════════${NC}"
echo ""

success "mkvauto has been installed successfully!"
echo ""
info "Configuration file: $CONFIG_DIR/config.yaml"
info "Output directory: $OUTPUT_DIR"
info "Optical drive: $DRIVE_PATH"
echo ""
info "Next steps:"
echo "  1. Customize your config: nano $CONFIG_DIR/config.yaml"
echo "  2. Create/import HandBrake presets to: $PRESETS_DIR/"
if [ "$NEEDS_RESTART" = true ]; then
    echo "  3. Restart your system (for group changes to take effect)"
    echo "  4. Run: mkvauto"
else
    echo "  3. Run: mkvauto"
fi
echo ""
info "Usage:"
echo "  mkvauto                          # Start the TUI"
echo "  mkvauto --add file.mkv           # Add file to queue"
echo ""
info "Controls (in TUI):"
echo "  A - Scan for missing encodes"
echo "  T - Retry failed items"
echo "  C - Clear completed items"
echo "  L - Toggle logs"
echo "  Q - Quit"
echo ""

if [ "$MAKEMKV_KEY" == "" ]; then
    warn "Remember to add your MakeMKV license key!"
    info "Edit ~/.MakeMKV/settings.conf and add:"
    info '  app_Key = "YOUR-KEY-HERE"'
    echo ""
fi

success "Happy ripping!"

# Prompt for restart if needed
if [ "$NEEDS_RESTART" = true ]; then
    echo ""
    warn "A system restart is required for optical drive access."
    read -p "Would you like to restart now? (y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        info "Restarting system..."
        sudo reboot
    else
        warn "Please restart your system before running mkvauto."
    fi
fi
