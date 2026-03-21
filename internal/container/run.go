package container

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/OpenAtom-Linyaps/linyaps/golang/ll-builder/internal/types"
)

// RunApplication runs a Linyaps application in a container
// Creates container with: base as rootfs, runtime at /runtime, app at /opt/apps/$appid/files
func RunApplication(config *types.ContainerConfig, args []string, env map[string]string, appBinary, runtimeBinary, baseDevelop, runtimeDevelop, workdir string) error {
	// Get base paths
	baseBinary := config.BasePath

	// Create overlay rootfs using same method as BuildInContainer
	rootfsDir, cleanup, err := CreateOverlayRootfs(baseBinary, baseDevelop)
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

	// Prepare runtime path - use overlay to merge binary and develop
	runtimeFiles := ""
	runtimeCleanup := func() {}
	if runtimeBinary != "" {
		overlayDir, cleanup, err := CreateOverlayRootfs(runtimeBinary, runtimeDevelop)
		if err == nil {
			runtimeFiles = overlayDir
			runtimeCleanup = cleanup
		}
	}
	defer runtimeCleanup()

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

	// Add GUI-related mounts
	mounts := ociConfig["mounts"].([]map[string]interface{})

	// X11 socket
	if _, err := os.Stat("/tmp/.X11-unix"); err == nil {
		mounts = append(mounts, map[string]interface{}{
			"destination": "/tmp/.X11-unix",
			"type":        "bind",
			"source":      "/tmp/.X11-unix",
			"options":     []string{"rbind", "rw"},
		})
	}

	// X11 authorization file
	homeDir := os.Getenv("HOME")
	if homeDir == "" {
		homeDir = "/root"
	}
	xauthority := filepath.Join(homeDir, ".Xauthority")
	if _, err := os.Stat(xauthority); err == nil {
		mounts = append(mounts, map[string]interface{}{
			"destination": xauthority,
			"type":        "bind",
			"source":      xauthority,
			"options":     []string{"rbind", "ro"},
		})
	}

	// GPU devices
	if _, err := os.Stat("/dev/dri"); err == nil {
		mounts = append(mounts, map[string]interface{}{
			"destination": "/dev/dri",
			"type":        "bind",
			"source":      "/dev/dri",
			"options":     []string{"rbind", "rw"},
		})
	}

	// Mount entire XDG_RUNTIME_DIR for D-Bus, Wayland, PulseAudio, etc.
	xdgRuntimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if xdgRuntimeDir == "" {
		xdgRuntimeDir = fmt.Sprintf("/run/user/%d", currentUID)
	}
	if _, err := os.Stat(xdgRuntimeDir); err == nil {
		mounts = append(mounts, map[string]interface{}{
			"destination": xdgRuntimeDir,
			"type":        "bind",
			"source":      xdgRuntimeDir,
			"options":     []string{"rbind", "rw"},
		})
	}
	busSocket := filepath.Join(xdgRuntimeDir, "bus")
	waylandDisplay := os.Getenv("WAYLAND_DISPLAY")

	ociConfig["mounts"] = mounts

	// Add display environment variables
	for i, env := range envList {
		if strings.HasPrefix(env, "DISPLAY=") || strings.HasPrefix(env, "WAYLAND_DISPLAY=") ||
			strings.HasPrefix(env, "XDG_RUNTIME_DIR=") || strings.HasPrefix(env, "DBUS_SESSION_BUS_ADDRESS=") ||
			strings.HasPrefix(env, "XAUTHORITY=") {
			envList = append(envList[:i], envList[i+1:]...)
		}
	}
	if display := os.Getenv("DISPLAY"); display != "" {
		envList = append(envList, "DISPLAY="+display)
	}
	if waylandDisplay != "" {
		envList = append(envList, "WAYLAND_DISPLAY="+waylandDisplay)
	}
	envList = append(envList, "XDG_RUNTIME_DIR="+xdgRuntimeDir)
	envList = append(envList, "DBUS_SESSION_BUS_ADDRESS=unix:path="+busSocket)
	if xauthority := os.Getenv("XAUTHORITY"); xauthority != "" {
		envList = append(envList, "XAUTHORITY="+xauthority)
	}

	// Update env in process config
	ociConfig["process"].(map[string]interface{})["env"] = envList

	// Add app files mount
	if appFiles != "" {
		appDir := "/opt/apps/" + config.AppID + "/files"
		ociConfig["mounts"] = append(ociConfig["mounts"].([]map[string]interface{}),
			map[string]interface{}{
				"destination": appDir,
				"type":        "bind",
				"source":      appFiles,
				"options":     []string{"rbind", "ro"},
			})
	} else {
		fmt.Println("Warning: appFiles is empty, skipping app mount")
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

	// Generate and mount ld.so.conf.d for library paths
	ldConfDir := filepath.Join("/tmp", fmt.Sprintf("ll-ldconf-%d-%d", os.Getpid(), time.Now().UnixNano()))
	os.MkdirAll(ldConfDir, 0755)
	ldConfPath := filepath.Join(ldConfDir, "zz_deepin-linglong-app.conf")
	ldConfContent := fmt.Sprintf(`/runtime/lib
/runtime/lib/x86_64-linux-gnu
/opt/apps/%s/files/lib
/opt/apps/%s/files/lib/x86_64-linux-gnu
`, config.AppID, config.AppID)
	if err := os.WriteFile(ldConfPath, []byte(ldConfContent), 0644); err == nil {
		ociConfig["mounts"] = append(ociConfig["mounts"].([]map[string]interface{}),
			map[string]interface{}{
				"destination": "/etc/ld.so.conf.d/zz_deepin-linglong-app.conf",
				"type":        "bind",
				"source":      ldConfPath,
				"options":     []string{"rbind", "ro"},
			})
		defer os.RemoveAll(ldConfDir)
	}

	// Mount host /etc/passwd and /etc/group for user info
	mounts = ociConfig["mounts"].([]map[string]interface{})
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
