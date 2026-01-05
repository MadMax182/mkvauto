#!/bin/bash

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored messages
info() { echo -e "${BLUE}[INFO]${NC} $1"; }
success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; }

echo -e "${YELLOW}════════════════════════════════════════════════════════════${NC}"
echo -e "${YELLOW}  MakeMKV Auto-Ripper Uninstaller${NC}"
echo -e "${YELLOW}════════════════════════════════════════════════════════════${NC}"
echo ""

warn "This will remove mkvauto from your system"
read -p "Do you want to continue? (y/N): " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    info "Uninstall cancelled"
    exit 0
fi

# Check if installed via pacman
if pacman -Qi mkvauto &> /dev/null; then
    info "Removing mkvauto package..."
    sudo pacman -Rns mkvauto

    # Ask to remove private repo
    read -p "Remove private repository from pacman.conf? (y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        sudo sed -i '/\[my-arch-repo\]/,/Server = file:/d' /etc/pacman.conf
        success "Removed repository from pacman.conf"

        read -p "Remove repository files from ~/.local/share/my-arch-repo? (y/N): " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            rm -rf ~/.local/share/my-arch-repo
            success "Removed repository files"
        fi
    fi
elif [ -f "/usr/local/bin/mkvauto" ]; then
    info "Removing from /usr/local/bin..."
    sudo rm -f /usr/local/bin/mkvauto
    success "Removed /usr/local/bin/mkvauto"
elif [ -f "$HOME/.local/bin/mkvauto" ]; then
    info "Removing from ~/.local/bin..."
    rm -f ~/.local/bin/mkvauto
    success "Removed ~/.local/bin/mkvauto"
else
    warn "mkvauto binary not found in common locations"
fi

# Ask to remove configuration
echo ""
read -p "Remove configuration files? (y/N): " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    read -p "Remove ~/.config/mkvauto? (y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        rm -rf ~/.config/mkvauto
        success "Removed ~/.config/mkvauto"
    fi

    read -p "Remove ~/.mkvauto (queue state)? (y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        rm -rf ~/.mkvauto
        success "Removed ~/.mkvauto"
    fi
fi

# Ask to remove output directory
echo ""
read -p "Remove output directory (encoded/ripped files)? (y/N): " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    if [ -f "$HOME/.config/mkvauto/config.yaml" ]; then
        OUTPUT_DIR=$(grep "output_dir:" ~/.config/mkvauto/config.yaml | cut -d'"' -f2)
        if [ -n "$OUTPUT_DIR" ] && [ -d "$OUTPUT_DIR" ]; then
            warn "This will delete: $OUTPUT_DIR"
            read -p "Are you sure? (y/N): " -n 1 -r
            echo
            if [[ $REPLY =~ ^[Yy]$ ]]; then
                rm -rf "$OUTPUT_DIR"
                success "Removed output directory"
            fi
        fi
    else
        warn "Could not determine output directory"
    fi
fi

# Ask to remove dependencies
echo ""
read -p "Remove dependencies (makemkv, handbrake-cli)? (y/N): " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    info "Removing dependencies..."

    if pacman -Qi makemkv &> /dev/null; then
        sudo pacman -Rns makemkv
    fi

    if pacman -Qi handbrake-cli &> /dev/null; then
        read -p "Remove handbrake-cli? (y/N): " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            sudo pacman -Rns handbrake-cli
        fi
    fi

    success "Dependencies removed"
fi

echo ""
success "Uninstall complete!"
info "Note: User group memberships (optical, video) were not removed"
info "To remove manually: sudo gpasswd -d $USER optical && sudo gpasswd -d $USER video"
