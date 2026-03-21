package repo

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/OpenAtom-Linyaps/linyaps/golang/ll-builder/internal/types"
)

const (
	// DefaultRepoURL is the default Linyaps repository URL
	DefaultRepoURL = "https://mirror-repo-linglong.deepin.com/repos/stable"
)

// OSTreeRepo represents an OSTree repository
type OSTreeRepo struct {
	// CacheDir is ~/.cache/linglong-builder/ostree
	CacheDir string
	// Config holds builder configuration
	Config *types.BuilderConfig
}

// NewOSTreeRepo creates a new OSTree repository
func NewOSTreeRepo(config *types.BuilderConfig) *OSTreeRepo {
	cacheDir := filepath.Join(os.Getenv("HOME"), ".cache", "linglong-builder", "ostree")
	os.MkdirAll(cacheDir, 0755)

	return &OSTreeRepo{
		CacheDir: cacheDir,
		Config:   config,
	}
}

// Init initializes the OSTree repository
func (r *OSTreeRepo) Init() error {
	repoDir := filepath.Join(r.CacheDir, "repo")

	// Check if repo already initialized
	if _, err := os.Stat(filepath.Join(repoDir, "config")); err == nil {
		return nil
	}

	// Create repo directory
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		return fmt.Errorf("failed to create repo dir: %w", err)
	}

	// Initialize ostree repo with bare-user-only mode
	cmd := exec.Command("ostree", "init", "--repo", repoDir, "--mode=bare-user-only")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to init ostree repo: %w", err)
	}

	// Add remote
	cmd = exec.Command("ostree", "remote", "add", "--repo", repoDir,
		"stable", DefaultRepoURL, "--no-gpg-verify")
	if err := cmd.Run(); err != nil {
		// Ignore if remote already exists
		fmt.Printf("Warning: failed to add remote: %v\n", err)
	}

	return nil
}

// Pull pulls a package from the repository and checks it out
func (r *OSTreeRepo) Pull(ref types.Reference, module string) error {
	if err := r.Init(); err != nil {
		return fmt.Errorf("failed to init repo: %w", err)
	}

	// Resolve version (e.g., 23.1.0 -> 23.1.0.3)
	resolvedRef, err := r.ResolveVersion(ref, module)
	if err != nil {
		// If resolution fails, try with original ref
		resolvedRef = &ref
	}

	// Version will be printed by ResolveVersion if different

	repoDir := filepath.Join(r.CacheDir, "repo")
	refStr := FormatOSTreeRef(*resolvedRef, module)

	fmt.Printf("Pulling %s from %s\n", refStr, DefaultRepoURL)

	// Pull from remote
	cmd := exec.Command("ostree", "pull", "--repo", repoDir, "stable", refStr)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to pull %s: %w", refStr, err)
	}

	// Get commit ID
	commitID, err := r.GetCommitID(*resolvedRef, module)
	if err != nil {
		return fmt.Errorf("failed to get commit id: %w", err)
	}

	// Checkout to layers/commit_id
	layersDir := filepath.Join(r.CacheDir, "layers")
	if err := os.MkdirAll(layersDir, 0755); err != nil {
		return fmt.Errorf("failed to create layers dir: %w", err)
	}

	destDir := filepath.Join(layersDir, commitID)
	if err := r.Checkout(*resolvedRef, module, destDir); err != nil {
		return fmt.Errorf("failed to checkout: %w", err)
	}

	fmt.Printf("Checked out to %s\n", destDir)
	return nil
}

// GetCommitID gets the commit ID for a ref
func (r *OSTreeRepo) GetCommitID(ref types.Reference, module string) (string, error) {
	repoDir := filepath.Join(r.CacheDir, "repo")
	refStr := FormatOSTreeRef(ref, module)

	cmd := exec.Command("ostree", "rev-parse", "--repo", repoDir, refStr)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to rev-parse %s: %w", refStr, err)
	}

	return strings.TrimSpace(string(output)), nil
}

// ResolveVersion resolves a partial version to the latest matching version
// For example: "23.1.0" -> "23.1.0.3" (if 23.1.0.2 and 23.1.0.3 exist)
func (r *OSTreeRepo) ResolveVersion(ref types.Reference, module string) (*types.Reference, error) {
	repoDir := filepath.Join(r.CacheDir, "repo")

	// Initialize if needed
	if err := r.Init(); err != nil {
		return &ref, nil // Return original ref if init fails
	}

	// List all remote refs
	cmd := exec.Command("ostree", "remote", "refs", "stable")
	cmd.Dir = repoDir
	output, err := cmd.Output()
	if err != nil {
		// If remote refs fails, return original ref
		return &ref, nil
	}

	// Build version prefix for matching
	// e.g., for version "23.1.0", match versions like "23.1.0.2", "23.1.0.3"
	// Use the original version string without padding zeros
	versionPrefix := fmt.Sprintf("%d.%d.%d", ref.Version.Major, ref.Version.Minor, ref.Version.Patch)

	// Find all matching versions
	var matchingVersions []string
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "stable:") {
			continue
		}

		// Extract version from ref
		// Format: stable:main/org.deepin.base/23.1.0.3/x86_64/binary
		// Remove "stable:" prefix
		refLine := strings.TrimPrefix(line, "stable:")
		parts := strings.Split(refLine, "/")
		if len(parts) < 5 {
			continue
		}

		// parts[0] = "main" (channel)
		// parts[1] = "org.deepin.base" (id)
		// parts[2] = "23.1.0.3" (version)
		// parts[3] = "x86_64" (arch)
		// parts[4] = "binary" (module)
		channel := parts[0]
		id := parts[1]
		versionStr := parts[2]
		arch := parts[3]
		refModule := parts[4]

		// Check if channel matches
		if channel != ref.Channel {
			continue
		}

		// Check if ID matches
		if id != ref.ID {
			continue
		}

		// Check if arch matches
		if arch != ref.Arch {
			continue
		}

		// Check if module matches
		if refModule != module {
			continue
		}

		// Check if version starts with the prefix
		if !strings.HasPrefix(versionStr, versionPrefix) {
			continue
		}

		matchingVersions = append(matchingVersions, versionStr)
	}

	if len(matchingVersions) == 0 {
		// No matching version found, return original
		return &ref, nil
	}

	// Find the latest version (highest)
	latestVersion := matchingVersions[0]
	for _, v := range matchingVersions[1:] {
		if compareVersions(v, latestVersion) > 0 {
			latestVersion = v
		}
	}

	// Only print if version was resolved
	if latestVersion != ref.Version.String() {
		fmt.Printf("Resolved version %s -> %s\n", ref.Version.String(), latestVersion)
	}

	// Parse the latest version
	latestVer, err := types.ParseVersion(latestVersion)
	if err != nil {
		return &ref, nil // Return original if parse fails
	}

	return &types.Reference{
		Channel: ref.Channel,
		ID:      ref.ID,
		Version: latestVer,
		Arch:    ref.Arch,
	}, nil
}

// compareVersions compares two version strings
// Returns: -1 if a < b, 0 if a == b, 1 if a > b
func compareVersions(a, b string) int {
	va, errA := types.ParseVersion(a)
	vb, errB := types.ParseVersion(b)

	if errA != nil || errB != nil {
		// Fall back to string comparison
		if a < b {
			return -1
		} else if a > b {
			return 1
		}
		return 0
	}

	if va.Major != vb.Major {
		return va.Major - vb.Major
	}
	if va.Minor != vb.Minor {
		return va.Minor - vb.Minor
	}
	if va.Patch != vb.Patch {
		return va.Patch - vb.Patch
	}
	return va.Tweak - vb.Tweak
}

// Checkout checks out a ref to a directory
func (r *OSTreeRepo) Checkout(ref types.Reference, module, destDir string) error {
	repoDir := filepath.Join(r.CacheDir, "repo")
	refStr := FormatOSTreeRef(ref, module)

	// Remove destination if exists
	os.RemoveAll(destDir)

	// Checkout with --user-mode to avoid permission issues
	cmd := exec.Command("ostree", "checkout", "--user-mode", "--repo", repoDir, refStr, destDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to checkout %s: %w", refStr, err)
	}

	return nil
}

// GetLayerDir returns the path to a checked out layer
// The layer is checked out to ~/.cache/linglong-builder/ostree/layers/$commit_id
func (r *OSTreeRepo) GetLayerDir(ref types.Reference, module string) (string, error) {
	// Resolve version to get the actual ref
	resolvedRef, err := r.ResolveVersion(ref, module)
	if err != nil {
		resolvedRef = &ref
	}

	// Get commit ID (may fail if not pulled yet)
	commitID, err := r.GetCommitID(*resolvedRef, module)
	if err == nil {
		// Check if layer already exists locally
		layerDir := filepath.Join(r.CacheDir, "layers", commitID)
		if _, err := os.Stat(layerDir); err == nil {
			return layerDir, nil
		}
	}

	// Pull if not exists locally
	if err := r.Pull(ref, module); err != nil {
		return "", err
	}

	// Get commit ID again after pull
	commitID, err = r.GetCommitID(*resolvedRef, module)
	if err != nil {
		return "", err
	}

	// Return the checkout directory
	layerDir := filepath.Join(r.CacheDir, "layers", commitID)
	if _, err := os.Stat(layerDir); err != nil {
		return "", fmt.Errorf("layer not found at %s", layerDir)
	}

	return layerDir, nil
}

// GetRootfs returns the rootfs path for a base reference
func (r *OSTreeRepo) GetRootfs(ref types.Reference) (string, error) {
	return r.GetLayerDir(ref, "binary")
}

// Exists checks if a ref exists locally
func (r *OSTreeRepo) Exists(ref types.Reference, module string) bool {
	repoDir := filepath.Join(r.CacheDir, "repo")
	refStr := FormatOSTreeRef(ref, module)

	cmd := exec.Command("ostree", "rev-parse", "--repo", repoDir, refStr)
	return cmd.Run() == nil
}

// GetModuleList returns available modules for a reference
func (r *OSTreeRepo) GetModuleList(ref types.Reference) []string {
	modules := []string{}
	for _, module := range []string{"binary", "develop"} {
		if r.Exists(ref, module) {
			modules = append(modules, module)
		}
	}
	return modules
}

// ImportLayerDir imports a layer directory into the local repository
func (r *OSTreeRepo) ImportLayerDir(layerDir string) error {
	// Read info.json
	infoPath := filepath.Join(layerDir, "info.json")
	infoData, err := os.ReadFile(infoPath)
	if err != nil {
		return fmt.Errorf("failed to read info.json: %w", err)
	}

	var info types.PackageInfoV2
	if err := json.Unmarshal(infoData, &info); err != nil {
		return fmt.Errorf("failed to parse info.json: %w", err)
	}

	// Create local layer directory
	version, err := types.ParseVersion(info.Version)
	if err != nil {
		return err
	}

	module := info.PackageInfoV2Module
	if module == "" {
		module = "binary"
	}

	localLayerDir := filepath.Join(r.CacheDir, "layers", info.ID, version.String(), module)

	// Copy layer directory
	if err := copyDir(layerDir, localLayerDir); err != nil {
		return fmt.Errorf("failed to copy layer: %w", err)
	}

	return nil
}

// FormatOSTreeRef formats a reference for OSTree
func FormatOSTreeRef(ref types.Reference, module string) string {
	return fmt.Sprintf("%s/%s/%s/%s/%s",
		ref.Channel,
		ref.ID,
		ref.Version.String(),
		ref.Arch,
		module,
	)
}

// copyDir copies a directory recursively
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		// Copy file
		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			return err
		}

		dstFile, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		defer dstFile.Close()

		_, err = dstFile.ReadFrom(srcFile)
		return err
	})
}

// ListLocal lists all local packages
func (r *OSTreeRepo) ListLocal() ([]types.LayerItem, error) {
	layersDir := filepath.Join(r.CacheDir, "layers")
	if _, err := os.Stat(layersDir); os.IsNotExist(err) {
		return nil, nil
	}

	var items []types.LayerItem

	packages, err := os.ReadDir(layersDir)
	if err != nil {
		return nil, err
	}

	for _, pkg := range packages {
		if !pkg.IsDir() {
			continue
		}

		pkgDir := filepath.Join(layersDir, pkg.Name())
		versions, err := os.ReadDir(pkgDir)
		if err != nil {
			continue
		}

		for _, ver := range versions {
			if !ver.IsDir() {
				continue
			}

			infoPath := filepath.Join(pkgDir, ver.Name(), "binary", "info.json")
			data, err := os.ReadFile(infoPath)
			if err != nil {
				continue
			}

			var info types.PackageInfoV2
			if err := json.Unmarshal(data, &info); err != nil {
				continue
			}

			items = append(items, types.LayerItem{
				Info: info,
			})
		}
	}

	return items, nil
}
