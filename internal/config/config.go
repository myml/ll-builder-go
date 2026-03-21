package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/OpenAtom-Linyaps/linyaps/golang/ll-builder/internal/types"
)

// DefaultConfigDir returns the default configuration directory
func DefaultConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("/tmp", ".linglong")
	}
	return filepath.Join(home, ".linglong")
}

// DefaultBuilderConfigDir returns the default builder configuration directory
func DefaultBuilderConfigDir() string {
	return filepath.Join(DefaultConfigDir(), "builder")
}

// DefaultRepoDir returns the default repository directory
func DefaultRepoDir() string {
	return filepath.Join(DefaultBuilderConfigDir(), "repo")
}

// LoadProject loads a BuilderProject from a YAML file
func LoadProject(path string) (*types.BuilderProject, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read project file: %w", err)
	}

	var project types.BuilderProject
	if err := yaml.Unmarshal(data, &project); err != nil {
		return nil, fmt.Errorf("failed to parse project file: %w", err)
	}

	// Validate version format
	if err := validateProject(&project); err != nil {
		return nil, err
	}

	return &project, nil
}

// SaveProject saves a BuilderProject to a YAML file
func SaveProject(path string, project *types.BuilderProject) error {
	data, err := yaml.Marshal(project)
	if err != nil {
		return fmt.Errorf("failed to marshal project: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write project file: %w", err)
	}

	return nil
}

// validateProject validates the project configuration
func validateProject(project *types.BuilderProject) error {
	// Validate version
	if project.Version != "1" {
		return fmt.Errorf("unsupported version: %s", project.Version)
	}

	// Validate package ID
	if project.Package.ID == "" {
		return fmt.Errorf("package.id is required")
	}

	// Validate package version format
	if _, err := types.ParseVersion(project.Package.Version); err != nil {
		return fmt.Errorf("invalid package.version: %w", err)
	}

	// Validate base field
	if project.Base == "" {
		return fmt.Errorf("base field is required")
	}

	// Validate command for app kind
	if project.Package.Kind == "app" && len(project.Command) == 0 {
		return fmt.Errorf("command field is required for app kind")
	}

	// Validate modules
	if project.Modules != nil {
		for _, module := range project.Modules {
			if module.Name == "binary" {
				return fmt.Errorf("configuration of binary modules is not allowed")
			}
		}
	}

	return nil
}

// LoadConfig loads builder configuration from file
func LoadConfig() (*types.BuilderConfig, error) {
	configPath := filepath.Join(DefaultBuilderConfigDir(), "config.yaml")

	// Default configuration
	cfg := &types.BuilderConfig{
		Repo: DefaultRepoDir(),
	}

	// Try to load from file
	if _, err := os.Stat(configPath); err == nil {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}

		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("failed to parse config file: %w", err)
		}
	}

	return cfg, nil
}

// SaveConfig saves builder configuration to file
func SaveConfig(cfg *types.BuilderConfig) error {
	configPath := filepath.Join(DefaultBuilderConfigDir(), "config.yaml")

	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// LoadRepoConfig loads repository configuration
func LoadRepoConfig(repoDir string) (*types.BuilderConfig, error) {
	configPath := filepath.Join(repoDir, "config.yaml")

	cfg := &types.BuilderConfig{
		Repo: repoDir,
	}

	if _, err := os.Stat(configPath); err == nil {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read repo config: %w", err)
		}

		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("failed to parse repo config: %w", err)
		}
	}

	return cfg, nil
}

// SaveRepoConfig saves repository configuration
func SaveRepoConfig(repoDir string, cfg *types.BuilderConfig) error {
	configPath := filepath.Join(repoDir, "config.yaml")

	if err := os.MkdirAll(repoDir, 0755); err != nil {
		return fmt.Errorf("failed to create repo directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal repo config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write repo config: %w", err)
	}

	return nil
}

// GetProjectYAMLPath finds the project YAML file
func GetProjectYAMLPath(projectDir string, usePath string) (string, error) {
	if usePath != "" {
		absPath, err := filepath.Abs(usePath)
		if err != nil {
			return "", fmt.Errorf("invalid file path: %w", err)
		}
		return absPath, nil
	}

	// Try architecture-specific file first
	arch := getCurrentArchitecture()
	archPath := filepath.Join(projectDir, fmt.Sprintf("linglong.%s.yaml", arch))
	if _, err := os.Stat(archPath); err == nil {
		return archPath, nil
	}

	// Try default file
	defaultPath := filepath.Join(projectDir, "linglong.yaml")
	if _, err := os.Stat(defaultPath); err == nil {
		return defaultPath, nil
	}

	return "", fmt.Errorf("project yaml file not found in %s", projectDir)
}

// getCurrentArchitecture returns the current CPU architecture
func getCurrentArchitecture() string {
	// This is a simplified implementation
	// In production, you would use runtime.GOARCH or similar
	return "x86_64"
}
