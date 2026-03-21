package repo

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/OpenAtom-Linyaps/linyaps/golang/ll-builder/internal/types"
	"gopkg.in/yaml.v3"
)

// LocalOSTreeRepo represents a local OSTree repository for storing built packages
type LocalOSTreeRepo struct {
	rootDir string
	config  *types.BuilderConfig
}

// NewLocalOSTreeRepo creates a new local OSTree repository
func NewLocalOSTreeRepo(rootDir string, config *types.BuilderConfig) *LocalOSTreeRepo {
	if rootDir == "" {
		rootDir = filepath.Join(os.Getenv("HOME"), ".cache", "linglong-builder", "local")
	}
	os.MkdirAll(rootDir, 0755)

	return &LocalOSTreeRepo{
		rootDir: rootDir,
		config:  config,
	}
}

// GetConfig returns the repository configuration
func (r *LocalOSTreeRepo) GetConfig() *types.BuilderConfig {
	return r.config
}

// SetConfig sets the repository configuration
func (r *LocalOSTreeRepo) SetConfig(config *types.BuilderConfig) error {
	r.config = config
	configPath := filepath.Join(r.rootDir, "config.yaml")

	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.MkdirAll(r.rootDir, 0755); err != nil {
		return fmt.Errorf("failed to create repo directory: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// GetDefaultRepo returns the default repository name
func (r *LocalOSTreeRepo) GetDefaultRepo() string {
	if r.config.DefaultRepo != "" {
		return r.config.DefaultRepo
	}
	return "stable"
}

// GetLayerDir returns the layer directory for a reference and module
func (r *LocalOSTreeRepo) GetLayerDir(ref types.Reference, module string) (string, error) {
	layerPath := filepath.Join(r.rootDir, "layers", ref.ID, ref.Version.String(), module)
	if _, err := os.Stat(layerPath); err != nil {
		return "", fmt.Errorf("layer not found: %s/%s/%s", ref.ID, ref.Version.String(), module)
	}
	return layerPath, nil
}

// GetModuleList returns the list of modules for a reference
func (r *LocalOSTreeRepo) GetModuleList(ref types.Reference) []string {
	packageDir := filepath.Join(r.rootDir, "layers", ref.ID, ref.Version.String())
	entries, err := os.ReadDir(packageDir)
	if err != nil {
		return nil
	}

	var modules []string
	for _, entry := range entries {
		if entry.IsDir() {
			modules = append(modules, entry.Name())
		}
	}

	sort.Strings(modules)
	return modules
}

// ImportLayerDir imports a layer directory into the repository
func (r *LocalOSTreeRepo) ImportLayerDir(layerDir string) error {
	// Read package info
	infoPath := filepath.Join(layerDir, "info.json")
	data, err := os.ReadFile(infoPath)
	if err != nil {
		return fmt.Errorf("failed to read package info: %w", err)
	}

	var info types.PackageInfoV2
	if err := json.Unmarshal(data, &info); err != nil {
		return fmt.Errorf("failed to parse package info: %w", err)
	}

	// Parse version
	version, err := types.ParseVersion(info.Version)
	if err != nil {
		return fmt.Errorf("invalid version: %w", err)
	}

	// Create target directory
	module := info.PackageInfoV2Module
	if module == "" {
		module = "binary"
	}

	targetDir := filepath.Join(r.rootDir, "layers", info.ID, version.String(), module)
	if err := os.MkdirAll(filepath.Dir(targetDir), 0755); err != nil {
		return fmt.Errorf("failed to create layer directory: %w", err)
	}

	// Remove existing layer if exists
	if _, err := os.Stat(targetDir); err == nil {
		if err := os.RemoveAll(targetDir); err != nil {
			return fmt.Errorf("failed to remove existing layer: %w", err)
		}
	}

	// Copy layer directory
	if err := copyDir(layerDir, targetDir); err != nil {
		return fmt.Errorf("failed to copy layer: %w", err)
	}

	return nil
}

// Remove removes a reference and module from the repository
func (r *LocalOSTreeRepo) Remove(ref types.Reference, module string) error {
	layerDir := filepath.Join(r.rootDir, "layers", ref.ID, ref.Version.String(), module)
	if _, err := os.Stat(layerDir); os.IsNotExist(err) {
		return fmt.Errorf("layer not found: %s/%s/%s", ref.ID, ref.Version.String(), module)
	}

	if err := os.RemoveAll(layerDir); err != nil {
		return fmt.Errorf("failed to remove layer: %w", err)
	}

	// Remove version directory if empty
	versionDir := filepath.Join(r.rootDir, "layers", ref.ID, ref.Version.String())
	entries, err := os.ReadDir(versionDir)
	if err == nil && len(entries) == 0 {
		os.RemoveAll(versionDir)
	}

	// Remove package directory if empty
	packageDir := filepath.Join(r.rootDir, "layers", ref.ID)
	entries, err = os.ReadDir(packageDir)
	if err == nil && len(entries) == 0 {
		os.RemoveAll(packageDir)
	}

	return nil
}

// ListLocal lists all local packages
func (r *LocalOSTreeRepo) ListLocal() ([]types.LayerItem, error) {
	layersDir := filepath.Join(r.rootDir, "layers")
	if _, err := os.Stat(layersDir); os.IsNotExist(err) {
		return nil, nil
	}

	var items []types.LayerItem

	packages, err := os.ReadDir(layersDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read layers directory: %w", err)
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

			modules, err := os.ReadDir(filepath.Join(pkgDir, ver.Name()))
			if err != nil {
				continue
			}

			for _, mod := range modules {
				if !mod.IsDir() {
					continue
				}

				infoPath := filepath.Join(pkgDir, ver.Name(), mod.Name(), "info.json")
				data, err := os.ReadFile(infoPath)
				if err != nil {
					continue
				}

				var info types.PackageInfoV2
				if err := json.Unmarshal(data, &info); err != nil {
					continue
				}

				items = append(items, types.LayerItem{
					Info:      info,
					Timestamp: time.Now().Unix(),
				})
			}
		}
	}

	return items, nil
}

// Push pushes a reference and module to the remote repository
func (r *LocalOSTreeRepo) Push(ref types.Reference, module string) error {
	return fmt.Errorf("push to remote not implemented")
}

// MergeModules merges module layers
func (r *LocalOSTreeRepo) MergeModules() error {
	return nil
}

// Prune removes unused objects from the repository
func (r *LocalOSTreeRepo) Prune() error {
	return nil
}

// ParseReference parses a reference string
func ParseReference(ref string) (*types.Reference, error) {
	return types.ParseReference(ref)
}
