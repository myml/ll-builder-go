package container

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/OpenAtom-Linyaps/linyaps/golang/ll-builder/internal/types"
)

// BuildInContainer runs build commands in a container using crun
// Uses overlayfs to merge base binary and develop layers
// buildOutput is mounted to /opt/apps/$appid/files
func BuildInContainer(config *types.ContainerConfig, commands []string, env map[string]string) error {
	// Find project directory
	projectDir := ""
	for _, m := range config.Mounts {
		if m.Destination == "/project" {
			projectDir = m.Source
			break
		}
	}
	if projectDir == "" {
		var err error
		projectDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get working directory: %w", err)
		}
	}

	// Find build output directory
	buildOutput := ""
	for _, m := range config.Mounts {
		if m.Destination == "/output" {
			buildOutput = m.Source
			break
		}
	}

	// Get base paths
	baseBinary := config.BasePath
	baseDevelop := ""
	if config.RuntimePath != nil && *config.RuntimePath != "" {
		baseDevelop = *config.RuntimePath
	}

	// Create overlay rootfs
	rootfsDir, cleanup, err := createOverlayRootfs(baseBinary, baseDevelop)
	if err != nil {
		return fmt.Errorf("failed to create overlay rootfs: %w", err)
	}
	defer cleanup()

	// Create container bundle
	containerID := fmt.Sprintf("ll-build-%d", os.Getpid())
	bundleDir := filepath.Join("/tmp", containerID)

	os.RemoveAll(bundleDir)
	if err := os.MkdirAll(bundleDir, 0755); err != nil {
		return fmt.Errorf("failed to create bundle dir: %w", err)
	}
	defer os.RemoveAll(bundleDir)

	// Create rootfs symlink
	if err := os.Symlink(rootfsDir, filepath.Join(bundleDir, "rootfs")); err != nil {
		return fmt.Errorf("failed to create rootfs symlink: %w", err)
	}

	// Prepare environment
	envList := []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"TERM=xterm",
		"HOME=/root",
		"LANG=C.UTF-8",
		"LINGLONG_APPID=" + config.AppID,
	}
	for k, v := range env {
		envList = append(envList, fmt.Sprintf("%s=%s", k, v))
	}

	// Prepare command
	cmdStr := strings.Join(commands, " && ")

	// Generate config.json
	ociConfig := map[string]interface{}{
		"ociVersion": "1.0.2",
		"hostname":   "ll-builder",
		"process": map[string]interface{}{
			"terminal": false,
			"user": map[string]interface{}{
				"uid": 0,
				"gid": 0,
			},
			"args": []string{"/bin/bash", "-c", cmdStr},
			"env":  envList,
			"cwd":  "/project",
			"capabilities": map[string]interface{}{
				"bounding":    []string{"CAP_CHOWN", "CAP_DAC_OVERRIDE", "CAP_FOWNER", "CAP_KILL", "CAP_SETGID", "CAP_SETUID", "CAP_SYS_CHROOT"},
				"effective":   []string{"CAP_CHOWN", "CAP_DAC_OVERRIDE", "CAP_FOWNER", "CAP_KILL", "CAP_SETGID", "CAP_SETUID", "CAP_SYS_CHROOT"},
				"inheritable": []string{"CAP_CHOWN", "CAP_DAC_OVERRIDE", "CAP_FOWNER", "CAP_KILL", "CAP_SETGID", "CAP_SETUID", "CAP_SYS_CHROOT"},
				"permitted":   []string{"CAP_CHOWN", "CAP_DAC_OVERRIDE", "CAP_FOWNER", "CAP_KILL", "CAP_SETGID", "CAP_SETUID", "CAP_SYS_CHROOT"},
			},
			"noNewPrivileges": true,
		},
		"root": map[string]interface{}{
			"path":     rootfsDir,
			"readonly": false,
		},
		"mounts": []map[string]interface{}{
			{"destination": "/proc", "type": "proc", "source": "proc"},
			{"destination": "/dev", "type": "tmpfs", "source": "tmpfs", "options": []string{"nosuid", "strictatime", "mode=755", "size=65536k"}},
			{"destination": "/dev/pts", "type": "devpts", "source": "devpts", "options": []string{"nosuid", "noexec", "newinstance", "ptmxmode=0666", "mode=0620", "gid=5"}},
			{"destination": "/dev/shm", "type": "tmpfs", "source": "shm", "options": []string{"nosuid", "noexec", "nodev", "mode=1777"}},
			{"destination": "/sys", "type": "sysfs", "source": "sysfs", "options": []string{"nosuid", "noexec", "nodev", "ro"}},
			{"destination": "/tmp", "type": "tmpfs", "source": "tmpfs", "options": []string{"nosuid", "nodev"}},
			{"destination": "/project", "type": "bind", "source": projectDir, "options": []string{"rbind", "rw"}},
			{"destination": "/opt/apps/" + config.AppID + "/files", "type": "bind", "source": buildOutput, "options": []string{"rbind", "rw"}},
		},
		"linux": map[string]interface{}{
			"namespaces": []map[string]string{
				{"type": "pid"},
				{"type": "ipc"},
				{"type": "uts"},
				{"type": "mount"},
			},
			"maskedPaths": []string{
				"/proc/acpi", "/proc/kcore", "/proc/keys",
				"/proc/latency_stats", "/proc/timer_list",
				"/proc/timer_stats", "/proc/sched_debug",
				"/sys/firmware",
			},
			"readonlyPaths": []string{
				"/proc/asound", "/proc/bus", "/proc/fs",
				"/proc/irq", "/proc/sys", "/proc/sysrq-trigger",
			},
		},
	}

	configData, err := json.MarshalIndent(ociConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	configPath := filepath.Join(bundleDir, "config.json")
	if err := os.WriteFile(configPath, configData, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	// Find crun
	crunPath, err := exec.LookPath("crun")
	if err != nil {
		return fmt.Errorf("crun not found: %w", err)
	}

	// Set XDG_RUNTIME_DIR for rootless mode
	if os.Getenv("XDG_RUNTIME_DIR") == "" {
		xdgDir := filepath.Join(os.Getenv("HOME"), ".run")
		os.MkdirAll(xdgDir, 0700)
		os.Setenv("XDG_RUNTIME_DIR", xdgDir)
	}

	// Run container
	fmt.Printf("Running container with overlay rootfs: %s\n", rootfsDir)
	cmd := exec.Command(crunPath, "run", "--bundle", bundleDir, containerID)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("container run failed: %w", err)
	}

	return nil
}

// RunApplication runs a Linyaps application in a container
// Creates container with: base as rootfs, runtime at /runtime, app at /opt/apps/$appid/files
func RunApplication(config *types.ContainerConfig, args []string, env map[string]string, appBinary, runtimeBinary, baseDevelop, runtimeDevelop, workdir string) error {
	// Get base paths
	baseBinary := config.BasePath

	// Create overlay rootfs using same method as BuildInContainer
	rootfsDir, cleanup, err := createOverlayRootfs(baseBinary, baseDevelop)
	if err != nil {
		return fmt.Errorf("failed to create overlay rootfs: %w", err)
	}
	defer cleanup()

	// Create container bundle
	containerID := fmt.Sprintf("ll-run-%d", os.Getpid())
	bundleDir := filepath.Join("/tmp", containerID)

	os.RemoveAll(bundleDir)
	if err := os.MkdirAll(bundleDir, 0755); err != nil {
		return fmt.Errorf("failed to create bundle dir: %w", err)
	}
	defer os.RemoveAll(bundleDir)

	// Create rootfs symlink
	if err := os.Symlink(rootfsDir, filepath.Join(bundleDir, "rootfs")); err != nil {
		return fmt.Errorf("failed to create rootfs symlink: %w", err)
	}

	// Prepare environment
	currentUser, _ := user.Current()
	userName := "user"
	if currentUser != nil {
		userName = currentUser.Username
	}
	envList := []string{
		"TERM=xterm",
		"USER=" + userName,
		"LC_ALL=C.UTF-8",
		"LANGUAGE=en_US:en",
		"LINGLONG_APPID=" + config.AppID,
	}
	if home := os.Getenv("HOME"); home != "" {
		envList = append(envList, "HOME="+home)
	}
	for k, v := range env {
		envList = append(envList, fmt.Sprintf("%s=%s", k, v))
	}

	// Prepare app files path
	appFiles := ""
	if appBinary != "" {
		appFiles = filepath.Join(appBinary, "files")
		if _, err := os.Stat(appFiles); err != nil {
			appFiles = ""
		}
	}

	// Prepare runtime path
	runtimeFiles := ""
	if runtimeBinary != "" {
		runtimeFiles = filepath.Join(runtimeBinary, "files")
		if _, err := os.Stat(runtimeFiles); err != nil {
			runtimeFiles = ""
		}
	}

	// Prepare command
	cmdStr := strings.Join(args, " ")

	// Set working directory
	cwd := "/"
	if workdir != "" {
		cwd = workdir
	}

	// Generate config.json - run as current host user with user namespace for rootless support
	currentUID := os.Getuid()
	currentGID := os.Getgid()
	ociConfig := map[string]interface{}{
		"ociVersion": "1.0.2",
		"hostname":   "ll-app",
		"process": map[string]interface{}{
			"terminal": false,
			"user": map[string]interface{}{
				"uid": currentUID,
				"gid": currentGID,
			},
			"args": []string{"/bin/bash", "--login", "-c", cmdStr},
			"env":  envList,
			"cwd":  cwd,
			"capabilities": map[string]interface{}{
				"bounding":    []string{"CAP_CHOWN", "CAP_DAC_OVERRIDE", "CAP_FOWNER", "CAP_KILL", "CAP_SYS_CHROOT"},
				"effective":   []string{"CAP_CHOWN", "CAP_DAC_OVERRIDE", "CAP_FOWNER", "CAP_KILL", "CAP_SYS_CHROOT"},
				"inheritable": []string{"CAP_CHOWN", "CAP_DAC_OVERRIDE", "CAP_FOWNER", "CAP_KILL", "CAP_SYS_CHROOT"},
				"permitted":   []string{"CAP_CHOWN", "CAP_DAC_OVERRIDE", "CAP_FOWNER", "CAP_KILL", "CAP_SYS_CHROOT"},
			},
			"noNewPrivileges": true,
		},
		"root": map[string]interface{}{
			"path":     rootfsDir,
			"readonly": false,
		},
		"mounts": []map[string]interface{}{
			{"destination": "/proc", "type": "proc", "source": "proc"},
			{"destination": "/dev", "type": "tmpfs", "source": "tmpfs", "options": []string{"nosuid", "strictatime", "mode=755", "size=65536k"}},
			{"destination": "/dev/pts", "type": "devpts", "source": "devpts", "options": []string{"nosuid", "noexec", "mode=0620"}},
			{"destination": "/dev/shm", "type": "tmpfs", "source": "shm", "options": []string{"nosuid", "noexec", "nodev", "mode=1777"}},
			{"destination": "/sys", "type": "sysfs", "source": "sysfs", "options": []string{"nosuid", "noexec", "nodev", "ro"}},
			{"destination": "/tmp", "type": "tmpfs", "source": "tmpfs", "options": []string{"nosuid", "nodev"}},
		},
		"linux": map[string]interface{}{
			"namespaces": []map[string]string{
				{"type": "pid"},
				{"type": "ipc"},
				{"type": "uts"},
				{"type": "mount"},
				{"type": "user"},
			},
			"uidMappings": []map[string]interface{}{
				{"containerID": currentUID, "hostID": currentUID, "size": 1},
			},
			"gidMappings": []map[string]interface{}{
				{"containerID": currentGID, "hostID": currentGID, "size": 1},
			},
			"maskedPaths": []string{
				"/proc/acpi", "/proc/kcore", "/proc/keys",
				"/proc/latency_stats", "/proc/timer_list",
				"/proc/timer_stats", "/proc/sched_debug",
				"/sys/firmware",
			},
			"readonlyPaths": []string{
				"/proc/asound", "/proc/bus", "/proc/fs",
				"/proc/irq", "/proc/sys", "/proc/sysrq-trigger",
			},
		},
	}

	// Add app files mount
	if appFiles != "" {
		ociConfig["mounts"] = append(ociConfig["mounts"].([]map[string]interface{}),
			map[string]interface{}{
				"destination": "/opt/apps/" + config.AppID + "/files",
				"type":        "bind",
				"source":      appFiles,
				"options":     []string{"rbind", "ro"},
			})
	}

	// Add runtime mount
	if runtimeFiles != "" {
		ociConfig["mounts"] = append(ociConfig["mounts"].([]map[string]interface{}),
			map[string]interface{}{
				"destination": "/runtime",
				"type":        "bind",
				"source":      runtimeFiles,
				"options":     []string{"rbind", "ro"},
			})
	}

	// Mount host /etc/passwd and /etc/group for user info
	mounts := ociConfig["mounts"].([]map[string]interface{})
	for _, f := range []string{"/etc/passwd", "/etc/group"} {
		if _, err := os.Stat(f); err == nil {
			mounts = append(mounts, map[string]interface{}{
				"destination": f,
				"type":        "bind",
				"source":      f,
				"options":     []string{"rbind", "ro"},
			})
		}
	}
	ociConfig["mounts"] = mounts

	configData, err := json.MarshalIndent(ociConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	configPath := filepath.Join(bundleDir, "config.json")
	if err := os.WriteFile(configPath, configData, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	// Find crun
	crunPath, err := exec.LookPath("crun")
	if err != nil {
		return fmt.Errorf("crun not found: %w", err)
	}

	// Set XDG_RUNTIME_DIR for rootless mode
	if os.Getenv("XDG_RUNTIME_DIR") == "" {
		xdgDir := filepath.Join(os.Getenv("HOME"), ".run")
		os.MkdirAll(xdgDir, 0700)
		os.Setenv("XDG_RUNTIME_DIR", xdgDir)
	}

	// Run container
	fmt.Printf("Running %s in container\n", config.AppID)
	cmd := exec.Command(crunPath, "run", "--bundle", bundleDir, containerID)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("container run failed: %w", err)
	}

	return nil
}

// createOverlayRootfs creates an overlayfs with base binary and develop layers
// Returns the merged directory path and a cleanup function
func createOverlayRootfs(baseBinary, baseDevelop string) (string, func(), error) {
	// Create overlay directories
	overlayDir := filepath.Join("/tmp", fmt.Sprintf("ll-overlay-%d", os.Getpid()))
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
