package disk

import (
	"fmt"
	"os/exec"
	"syscall"
)

const (
	CDROMEJECT = 0x5309 // ioctl command to eject CD/DVD
)

// Eject ejects the disc from the specified device
func Eject(devicePath string) error {
	// Try using the eject command first (more reliable)
	cmd := exec.Command("eject", devicePath)
	if err := cmd.Run(); err == nil {
		return nil
	}

	// Fallback to ioctl if eject command fails
	return ejectViaIoctl(devicePath)
}

// ejectViaIoctl uses ioctl system call to eject the disc
func ejectViaIoctl(devicePath string) error {
	fd, err := syscall.Open(devicePath, syscall.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return fmt.Errorf("failed to open device: %w", err)
	}
	defer syscall.Close(fd)

	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(fd),
		uintptr(CDROMEJECT),
		0,
	)

	if errno != 0 {
		return fmt.Errorf("ioctl eject failed: %v", errno)
	}

	return nil
}

// Close closes the disc tray (opposite of eject)
func Close(devicePath string) error {
	cmd := exec.Command("eject", "-t", devicePath)
	return cmd.Run()
}
