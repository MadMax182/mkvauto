package disk

import (
	"context"
	"syscall"
	"time"
)

const (
	// ioctl constants for CD/DVD drive
	CDROM_DRIVE_STATUS = 0x5326
	CDS_NO_INFO        = 0
	CDS_NO_DISC        = 1
	CDS_TRAY_OPEN      = 2
	CDS_DRIVE_NOT_READY = 3
	CDS_DISC_OK        = 4
)

type Detector struct {
	devicePath string
	pollInterval time.Duration
}

func NewDetector(devicePath string) *Detector {
	return &Detector{
		devicePath:   devicePath,
		pollInterval: 2 * time.Second,
	}
}

// Start begins monitoring for disc insertion
// Returns a channel that will receive detected discs
func (d *Detector) Start(ctx context.Context) <-chan DetectedDisc {
	ch := make(chan DetectedDisc, 1)

	go func() {
		defer close(ch)

		lastStatus := CDS_NO_DISC

		ticker := time.NewTicker(d.pollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				status, err := d.checkDriveStatus()
				if err != nil {
					// Drive not accessible, continue polling
					continue
				}

				// Detect transition from no disc to disc present
				if lastStatus != CDS_DISC_OK && status == CDS_DISC_OK {
					// Disc inserted! Wait a moment for it to settle
					time.Sleep(2 * time.Second)

					// Verify disc is still there
					status, err = d.checkDriveStatus()
					if err == nil && status == CDS_DISC_OK {
						ch <- DetectedDisc{
							Device: d.devicePath,
							// Name and DiscType will be populated by MakeMKV scan
						}
					}
				}

				lastStatus = status
			}
		}
	}()

	return ch
}

// checkDriveStatus uses ioctl to check if a disc is present
func (d *Detector) checkDriveStatus() (int, error) {
	// Open the device
	fd, err := syscall.Open(d.devicePath, syscall.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return CDS_NO_INFO, err
	}
	defer syscall.Close(fd)

	// Call ioctl to get drive status
	status, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(fd),
		uintptr(CDROM_DRIVE_STATUS),
		uintptr(0),
	)

	if errno != 0 {
		return CDS_NO_INFO, errno
	}

	return int(status), nil
}

// IsDiscPresent checks if a disc is currently in the drive
func (d *Detector) IsDiscPresent() bool {
	status, err := d.checkDriveStatus()
	return err == nil && status == CDS_DISC_OK
}
