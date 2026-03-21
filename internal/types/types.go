package types

import (
	"fmt"
	"strings"
)

// BuilderProject represents the linglong.yaml configuration
type BuilderProject struct {
	Version     string                  `yaml:"version" json:"version"`
	Package     BuilderProjectPackage   `yaml:"package" json:"package"`
	Command     []string                `yaml:"command,omitempty" json:"command,omitempty"`
	Base        string                  `yaml:"base" json:"base"`
	Runtime     *string                 `yaml:"runtime,omitempty" json:"runtime,omitempty"`
	Sources     []BuilderProjectSource  `yaml:"sources,omitempty" json:"sources,omitempty"`
	Build       *string                 `yaml:"build,omitempty" json:"build,omitempty"`
	Modules     []BuilderProjectModule  `yaml:"modules,omitempty" json:"modules,omitempty"`
	BuildExt    *BuilderProjectBuildExt `yaml:"buildext,omitempty" json:"buildext,omitempty"`
	Permissions *Permissions            `yaml:"permissions,omitempty" json:"permissions,omitempty"`
}

// BuilderProjectPackage represents package information
type BuilderProjectPackage struct {
	ID          string            `yaml:"id" json:"id"`
	Name        string            `yaml:"name" json:"name"`
	Version     string            `yaml:"version" json:"version"`
	Kind        string            `yaml:"kind" json:"kind"`
	Description string            `yaml:"description,omitempty" json:"description,omitempty"`
	DeviceNodes []string          `yaml:"deviceNodes,omitempty" json:"deviceNodes,omitempty"`
	Env         map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	Libs        []string          `yaml:"libs,omitempty" json:"libs,omitempty"`
}

// BuilderProjectSource represents source configuration
type BuilderProjectSource struct {
	Kind       string  `yaml:"kind" json:"kind"`
	URL        *string `yaml:"url,omitempty" json:"url,omitempty"`
	Digest     *string `yaml:"digest,omitempty" json:"digest,omitempty"`
	Commit     *string `yaml:"commit,omitempty" json:"commit,omitempty"`
	Name       *string `yaml:"name,omitempty" json:"name,omitempty"`
	Submodules *bool   `yaml:"submodules,omitempty" json:"submodules,omitempty"`
	Version    *string `yaml:"version,omitempty" json:"version,omitempty"`
}

// BuilderProjectModule represents module configuration
type BuilderProjectModule struct {
	Name  string   `yaml:"name" json:"name"`
	Rules []string `yaml:"rules,omitempty" json:"rules,omitempty"`
}

// BuilderProjectBuildExt represents extended build configuration
type BuilderProjectBuildExt struct {
	Apt *BuilderProjectApt `yaml:"apt,omitempty" json:"apt,omitempty"`
}

// BuilderProjectApt represents apt configuration
type BuilderProjectApt struct {
	BuildDepends []string `yaml:"build_depends,omitempty" json:"build_depends,omitempty"`
	Depends      []string `yaml:"depends,omitempty" json:"depends,omitempty"`
}

// Permissions represents application permissions
type Permissions struct {
}

// BuilderConfig represents builder configuration
type BuilderConfig struct {
	Repo        string       `yaml:"repo" json:"repo"`
	Repos       []RepoConfig `yaml:"repos,omitempty" json:"repos,omitempty"`
	DefaultRepo string       `yaml:"defaultRepo,omitempty" json:"defaultRepo,omitempty"`
	Offline     bool         `yaml:"offline,omitempty" json:"offline,omitempty"`
}

// RepoConfig represents repository configuration
type RepoConfig struct {
	Name          string  `yaml:"name" json:"name"`
	URL           string  `yaml:"url" json:"url"`
	Alias         *string `yaml:"alias,omitempty" json:"alias,omitempty"`
	MirrorEnabled *bool   `yaml:"mirrorEnabled,omitempty" json:"mirrorEnabled,omitempty"`
}

// PackageInfoV2 represents package information
type PackageInfoV2 struct {
	SchemaVersion       string         `json:"schemaVersion"`
	ID                  string         `json:"id"`
	Name                string         `json:"name"`
	Version             string         `json:"version"`
	Arch                []string       `json:"arch"`
	Kind                string         `json:"kind"`
	Description         string         `json:"description,omitempty"`
	Base                string         `json:"base"`
	Runtime             *string        `json:"runtime,omitempty"`
	Channel             *string        `json:"channel,omitempty"`
	Command             []string       `json:"command,omitempty"`
	Size                int64          `json:"size,omitempty"`
	Permissions         *Permissions   `json:"permissions,omitempty"`
	PackageInfoV2Module string         `json:"packageInfoV2Module,omitempty"`
	ExtImpl             *ExtensionImpl `json:"extImpl,omitempty"`
}

// ExtensionImpl represents extension implementation
type ExtensionImpl struct {
	DeviceNodes []string          `json:"deviceNodes,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	Libs        []string          `json:"libs,omitempty"`
}

// ExportOption represents export options
type ExportOption struct {
	IconPath        string   `json:"iconPath,omitempty"`
	Loader          string   `json:"loader,omitempty"`
	Compressor      string   `json:"compressor,omitempty"`
	Ref             string   `json:"ref,omitempty"`
	Modules         []string `json:"modules,omitempty"`
	NoExportDevelop bool     `json:"noExportDevelop,omitempty"`
}

// BuilderBuildOptions represents build options
type BuilderBuildOptions struct {
	FullDevelop      bool `json:"fullDevelop,omitempty"`
	SkipFetchSource  bool `json:"skipFetchSource,omitempty"`
	SkipPullDepend   bool `json:"skipPullDepend,omitempty"`
	SkipRunContainer bool `json:"skipRunContainer,omitempty"`
	SkipCommitOutput bool `json:"skipCommitOutput,omitempty"`
	SkipCheckOutput  bool `json:"skipCheckOutput,omitempty"`
	SkipStripSymbols bool `json:"skipStripSymbols,omitempty"`
	IsolateNetWork   bool `json:"isolateNetWork,omitempty"`
}

// Reference represents a package reference
// Format: channel/id/version/arch/module
type Reference struct {
	Channel string  `json:"channel"`
	ID      string  `json:"id"`
	Version Version `json:"version"`
	Arch    string  `json:"arch"`
	Module  string  `json:"module,omitempty"`
}

// String returns the string representation of the reference
// Format: channel/id/version/arch/module
func (r Reference) String() string {
	s := fmt.Sprintf("%s/%s/%s/%s", r.Channel, r.ID, r.Version.String(), r.Arch)
	if r.Module != "" {
		s += "/" + r.Module
	}
	return s
}

// Version represents a package version
type Version struct {
	Major int `json:"major"`
	Minor int `json:"minor"`
	Patch int `json:"patch"`
	Tweak int `json:"tweak"`
}

func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d.%d", v.Major, v.Minor, v.Patch, v.Tweak)
}

// ParseVersion parses a version string
// Supports formats: major.minor.patch.tweak or major.minor.patch (tweak defaults to 0)
func ParseVersion(s string) (Version, error) {
	var v Version

	// Try 4-part version first
	_, err := fmt.Sscanf(s, "%d.%d.%d.%d", &v.Major, &v.Minor, &v.Patch, &v.Tweak)
	if err == nil {
		return v, nil
	}

	// Try 3-part version
	_, err = fmt.Sscanf(s, "%d.%d.%d", &v.Major, &v.Minor, &v.Patch)
	if err == nil {
		v.Tweak = 0
		return v, nil
	}

	// Try 2-part version
	_, err = fmt.Sscanf(s, "%d.%d", &v.Major, &v.Minor)
	if err == nil {
		v.Patch = 0
		v.Tweak = 0
		return v, nil
	}

	// Try 1-part version
	_, err = fmt.Sscanf(s, "%d", &v.Major)
	if err == nil {
		v.Minor = 0
		v.Patch = 0
		v.Tweak = 0
		return v, nil
	}

	return v, fmt.Errorf("invalid version format: %s", s)
}

// ParseReference parses a reference string
// Format: channel/id/version/arch/module
func ParseReference(ref string) (*Reference, error) {
	parts := strings.Split(ref, "/")
	if len(parts) < 4 {
		return nil, fmt.Errorf("invalid reference format (expected channel/id/version/arch[/module]): %s", ref)
	}

	version, err := ParseVersion(parts[2])
	if err != nil {
		return nil, fmt.Errorf("invalid version in reference: %w", err)
	}

	r := &Reference{
		Channel: parts[0],
		ID:      parts[1],
		Version: version,
		Arch:    parts[3],
	}

	if len(parts) > 4 {
		r.Module = parts[4]
	}

	return r, nil
}

// FuzzyReference represents a fuzzy package reference
type FuzzyReference struct {
	ID      *string `json:"id,omitempty"`
	Version *string `json:"version,omitempty"`
	Channel *string `json:"channel,omitempty"`
	Arch    *string `json:"arch,omitempty"`
}

// ReferenceWithRepo represents a reference with repository
type ReferenceWithRepo struct {
	Repo      string    `json:"repo"`
	Reference Reference `json:"reference"`
}

// LayerItem represents a layer item in repository
type LayerItem struct {
	Info      PackageInfoV2 `json:"info"`
	Commit    string        `json:"commit"`
	Timestamp int64         `json:"timestamp"`
}

// ContainerConfig represents container configuration
type ContainerConfig struct {
	AppID       string            `json:"appId"`
	BasePath    string            `json:"basePath"`
	RuntimePath *string           `json:"runtimePath,omitempty"`
	Mounts      []Mount           `json:"mounts,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	Caps        []string          `json:"caps,omitempty"`
	Network     bool              `json:"network"`
	UID         int               `json:"uid"`
	GID         int               `json:"gid"`
}

// Mount represents a mount point
type Mount struct {
	Destination string   `json:"destination"`
	Source      string   `json:"source"`
	Type        string   `json:"type"`
	Options     []string `json:"options,omitempty"`
}

// Process represents a container process
type Process struct {
	Args      []string `json:"args"`
	Cwd       string   `json:"cwd"`
	Env       []string `json:"env,omitempty"`
	NoNewPriv bool     `json:"noNewPrivileges"`
	Terminal  bool     `json:"terminal"`
}
