package container

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
	rootfsDir, cleanup, err := CreateOverlayRootfs(baseBinary, baseDevelop)
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
	cmdStr := strings.Join(commands, " ")

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
			"args": []string{"/bin/bash", "--login", "-c", cmdStr},
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

	// Add extra mounts from config (e.g., /runtime)
	mounts := ociConfig["mounts"].([]map[string]interface{})
	for _, m := range config.Mounts {
		if m.Destination == "/project" || m.Destination == "/output" {
			continue
		}
		mounts = append(mounts, map[string]interface{}{
			"destination": m.Destination,
			"type":        m.Type,
			"source":      m.Source,
			"options":     m.Options,
		})
	}
	ociConfig["mounts"] = mounts

	// Add startContainer hook to run ldconfig
	ociConfig["hooks"] = map[string]interface{}{
		"startContainer": []map[string]interface{}{
			{
				"path": "/sbin/ldconfig",
				"args": []string{"ldconfig"},
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
