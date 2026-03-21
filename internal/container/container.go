package container

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// CreateOverlayRootfs creates an overlayfs with binary and develop layers
// Returns the merged directory path and a cleanup function
func CreateOverlayRootfs(baseBinary, baseDevelop string) (string, func(), error) {
	// Create overlay directories with unique name
	overlayDir := filepath.Join("/tmp", fmt.Sprintf("ll-overlay-%d-%d", os.Getpid(), time.Now().UnixNano()))
	upperDir := filepath.Join(overlayDir, "upper")
	workDir := filepath.Join(overlayDir, "work")
	mergedDir := filepath.Join(overlayDir, "merged")

	// Create directories
	for _, dir := range []string{upperDir, workDir, mergedDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", nil, fmt.Errorf("failed to create dir %s: %w", dir, err)
		}
	}

	if baseBinary == "" {
		return "", nil, fmt.Errorf("base binary path is required")
	}

	// Use files/ subdirectory as the actual rootfs
	binaryFiles := filepath.Join(baseBinary, "files")
	developFiles := ""
	if baseDevelop != "" {
		developFiles = filepath.Join(baseDevelop, "files")
	}

	// Check if files directory exists
	if _, err := os.Stat(binaryFiles); err != nil {
		// If files/ doesn't exist, use the base path directly
		binaryFiles = baseBinary
	}

	// Build lowerdir list: develop (if exists) as upper lower layer, binary as lower layer
	lowerStr := binaryFiles
	if developFiles != "" {
		if _, err := os.Stat(developFiles); err == nil {
			lowerStr = developFiles + ":" + binaryFiles
		}
	}

	// Try fuse-overlayfs first (for non-root users)
	fuseOverlayfs, err := exec.LookPath("fuse-overlayfs")
	if err == nil {
		cmd := exec.Command(fuseOverlayfs,
			"-o", fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", lowerStr, upperDir, workDir),
			mergedDir,
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err == nil {
			cleanup := func() {
				exec.Command("fusermount3", "-u", mergedDir).Run()
				os.RemoveAll(overlayDir)
			}
			return mergedDir, cleanup, nil
		}
	}

	// Try kernel overlayfs
	mountOpts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", lowerStr, upperDir, workDir)
	if err := syscall.Mount("overlay", mergedDir, "overlay", 0, mountOpts); err != nil {
		return "", nil, fmt.Errorf("failed to mount overlay: %w", err)
	}

	cleanup := func() {
		syscall.Unmount(mergedDir, 0)
		os.RemoveAll(overlayDir)
	}

	return mergedDir, cleanup, nil
}
