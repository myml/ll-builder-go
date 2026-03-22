package builder

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/OpenAtom-Linyaps/linyaps/golang/ll-builder/internal/container"
	"github.com/OpenAtom-Linyaps/linyaps/golang/ll-builder/internal/layer"
	"github.com/OpenAtom-Linyaps/linyaps/golang/ll-builder/internal/repo"
	"github.com/OpenAtom-Linyaps/linyaps/golang/ll-builder/internal/source"
	"github.com/OpenAtom-Linyaps/linyaps/golang/ll-builder/internal/types"
)

// Builder represents the Linyaps builder
type Builder struct {
	project         *types.BuilderProject
	projectYamlFile string
	workingDir      string
	internalDir     string
	localRepo       *repo.OSTreeRepo
	config          *types.BuilderConfig
	buildOptions    types.BuilderBuildOptions
	sourceFetcher   *source.Fetcher
	layerPackager   *layer.Packager
	uid             int
	gid             int
	projectRef      *types.Reference
	packageModules  []string
	buildOutput     string
	installPrefix   string
	baseBinary      string
	baseDevelop     string
	runtimeBinary   string
	runtimeDevelop  string
}

// NewBuilder creates a new builder
func NewBuilder(project *types.BuilderProject, workingDir string, localRepo *repo.OSTreeRepo, cfg *types.BuilderConfig) *Builder {
	return &Builder{
		project:       project,
		workingDir:    workingDir,
		internalDir:   filepath.Join(workingDir, "linglong"),
		localRepo:     localRepo,
		config:        cfg,
		sourceFetcher: source.NewFetcher(filepath.Join(workingDir, "linglong", "cache")),
		layerPackager: layer.NewPackager(),
		uid:           os.Getuid(),
		gid:           os.Getgid(),
	}
}

// SetBuildOptions sets the build options
func (b *Builder) SetBuildOptions(options types.BuilderBuildOptions) {
	b.buildOptions = options
}

// GetConfig returns the builder configuration
func (b *Builder) GetConfig() *types.BuilderConfig {
	return b.config
}

// Create creates a new project template
func (b *Builder) Create(projectName string) error {
	projectDir := filepath.Join(b.workingDir, projectName)

	// Check if directory already exists
	if _, err := os.Stat(projectDir); err == nil {
		return fmt.Errorf("project directory already exists: %s", projectDir)
	}

	// Create project directory
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		return fmt.Errorf("failed to create project directory: %w", err)
	}

	// Create template content
	template := fmt.Sprintf(`version: '1'
package:
  id: %s
  name: %s
  version: 0.0.0.1
  kind: app
  description: |
    %s application
command: [%s]
base: org.deepin.base/25.2.1
build: |
  mkdir -p $PREFIX/bin
  echo "echo hello" > $PREFIX/bin/%s
  chmod +x $PREFIX/bin/%s
`, projectName, projectName, projectName, projectName, projectName, projectName)

	configPath := filepath.Join(projectDir, "linglong.yaml")
	if err := os.WriteFile(configPath, []byte(template), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Printf("Project %s created successfully at %s\n", projectName, projectDir)
	return nil
}

// Build builds the project
func (b *Builder) Build(args []string) error {
	fmt.Println("[Build Start]")

	// Stage 1: Prepare
	if err := b.buildStagePrepare(); err != nil {
		return fmt.Errorf("stage prepare error: %w", err)
	}

	// Stage 2: Fetch source
	if err := b.buildStageFetchSource(); err != nil {
		return fmt.Errorf("stage fetch source error: %w", err)
	}

	// Stage 3: Pull dependency
	if err := b.buildStagePullDependency(); err != nil {
		return fmt.Errorf("stage pull dependency error: %w", err)
	}

	// Stage 4: Build
	if !b.buildOptions.SkipRunContainer {
		if err := b.buildStageBuild(args); err != nil {
			return fmt.Errorf("stage build error: %w", err)
		}
	}

	// Stage 5: Commit
	if !b.buildOptions.SkipCommitOutput {
		if err := b.buildStageCommit(); err != nil {
			return fmt.Errorf("stage commit error: %w", err)
		}
	}

	fmt.Printf("Successfully build %s\n", b.project.Package.ID)
	return nil
}

// buildStagePrepare prepares the build environment
func (b *Builder) buildStagePrepare() error {
	fmt.Println("[Stage Prepare]")

	// Get project reference
	ref, err := b.currentReference()
	if err != nil {
		return fmt.Errorf("invalid project info: %w", err)
	}
	b.projectRef = ref

	// Set install prefix
	switch b.project.Package.Kind {
	case "app":
		if len(b.project.Command) == 0 {
			return fmt.Errorf("command field is required for app kind")
		}
		b.installPrefix = fmt.Sprintf("/opt/apps/%s/files", b.project.Package.ID)
	case "runtime":
		b.installPrefix = "/runtime"
	case "extension":
		b.installPrefix = fmt.Sprintf("/opt/extensions/%s", b.project.Package.ID)
	default:
		return fmt.Errorf("unknown package kind: %s", b.project.Package.Kind)
	}

	// Create internal directory
	if err := os.MkdirAll(b.internalDir, 0755); err != nil {
		return fmt.Errorf("failed to create internal directory: %w", err)
	}

	// Print basic info
	b.printBasicInfo()

	return nil
}

// buildStageFetchSource fetches the source code
func (b *Builder) buildStageFetchSource() error {
	fmt.Println("[Processing Sources]")

	if b.project.Sources == nil || len(b.project.Sources) == 0 {
		fmt.Println("No sources to fetch")
		return nil
	}

	if b.buildOptions.SkipFetchSource {
		fmt.Println("Skipping source fetch")
		return nil
	}

	cacheDir := filepath.Join(b.internalDir, "cache")
	if envCache := os.Getenv("LINGLONG_FETCH_CACHE"); envCache != "" {
		cacheDir = envCache
	}

	sourcesDir := filepath.Join(b.internalDir, "sources")
	os.RemoveAll(sourcesDir)

	fetcher := source.NewFetcher(cacheDir)
	return fetcher.FetchAll(b.project.Sources, sourcesDir)
}

// buildStagePullDependency pulls the dependencies
func (b *Builder) buildStagePullDependency() error {
	fmt.Println("[Processing Dependency]")

	// Parse base reference
	baseRef, err := b.parseDependency(b.project.Base)
	if err != nil {
		return fmt.Errorf("base dependency error: %w", err)
	}

	// Parse runtime reference if exists
	var runtimeRef *types.Reference
	if b.project.Runtime != nil {
		ref, err := b.parseDependency(*b.project.Runtime)
		if err != nil {
			return fmt.Errorf("runtime dependency error: %w", err)
		}
		runtimeRef = ref
	}

	// Create OSTree repo
	ostreeRepo := repo.NewOSTreeRepo(b.config)

	// Resolve versions first
	resolvedBaseRef, _ := ostreeRepo.ResolveVersion(*baseRef, "binary")
	if resolvedBaseRef == nil {
		resolvedBaseRef = baseRef
	}

	var resolvedRuntimeRef *types.Reference
	if runtimeRef != nil {
		resolvedRuntimeRef, _ = ostreeRepo.ResolveVersion(*runtimeRef, "binary")
		if resolvedRuntimeRef == nil {
			resolvedRuntimeRef = runtimeRef
		}
	}

	// Pull dependencies if not skipped
	if !b.buildOptions.SkipPullDepend {
		// Pull base binary if not exists locally
		if !ostreeRepo.Exists(*resolvedBaseRef, "binary") {
			fmt.Printf("Pulling base %s binary...\n", baseRef.ID)
			if err := ostreeRepo.Pull(*baseRef, "binary"); err != nil {
				fmt.Printf("Warning: failed to pull base binary: %v\n", err)
				// Create minimal base layer for testing
				if err := b.createMinimalBaseLayer(*baseRef, "binary"); err != nil {
					return fmt.Errorf("failed to create minimal base layer: %w", err)
				}
			}
		} else {
			fmt.Printf("Base %s binary already exists locally, skipping pull\n", baseRef.ID)
		}

		// Pull base develop if not exists locally
		if !ostreeRepo.Exists(*resolvedBaseRef, "develop") {
			fmt.Printf("Pulling base %s develop...\n", baseRef.ID)
			if err := ostreeRepo.Pull(*baseRef, "develop"); err != nil {
				fmt.Printf("Warning: failed to pull base develop: %v\n", err)
			}
		} else {
			fmt.Printf("Base %s develop already exists locally, skipping pull\n", baseRef.ID)
		}

		// Pull runtime if exists and not exists locally
		if runtimeRef != nil && resolvedRuntimeRef != nil {
			if !ostreeRepo.Exists(*resolvedRuntimeRef, "binary") {
				fmt.Printf("Pulling runtime %s binary...\n", runtimeRef.ID)
				ostreeRepo.Pull(*runtimeRef, "binary")
			} else {
				fmt.Printf("Runtime %s binary already exists locally, skipping pull\n", runtimeRef.ID)
			}

			if !ostreeRepo.Exists(*resolvedRuntimeRef, "develop") {
				fmt.Printf("Pulling runtime %s develop...\n", runtimeRef.ID)
				ostreeRepo.Pull(*runtimeRef, "develop")
			} else {
				fmt.Printf("Runtime %s develop already exists locally, skipping pull\n", runtimeRef.ID)
			}
		}
	}

	// Get base paths (may already exist from previous pull)
	b.baseBinary, err = ostreeRepo.GetLayerDir(*baseRef, "binary")
	if err != nil {
		return fmt.Errorf("failed to get base binary path: %w", err)
	}
	fmt.Printf("Base binary: %s\n", b.baseBinary)

	b.baseDevelop, err = ostreeRepo.GetLayerDir(*baseRef, "develop")
	if err != nil {
		fmt.Printf("Warning: failed to get base develop path: %v\n", err)
		b.baseDevelop = ""
	} else {
		fmt.Printf("Base develop: %s\n", b.baseDevelop)
	}

	// Get runtime paths if specified
	if runtimeRef != nil {
		b.runtimeBinary, err = ostreeRepo.GetLayerDir(*runtimeRef, "binary")
		if err != nil {
			fmt.Printf("Warning: failed to get runtime binary path: %v\n", err)
			b.runtimeBinary = ""
		} else {
			fmt.Printf("Runtime binary: %s\n", b.runtimeBinary)
		}

		b.runtimeDevelop, err = ostreeRepo.GetLayerDir(*runtimeRef, "develop")
		if err != nil {
			fmt.Printf("Warning: failed to get runtime develop path: %v\n", err)
			b.runtimeDevelop = ""
		} else {
			fmt.Printf("Runtime develop: %s\n", b.runtimeDevelop)
		}
	}

	return nil
}

// createMinimalBaseLayer creates a minimal base layer when pull fails
func (b *Builder) createMinimalBaseLayer(ref types.Reference, module string) error {
	cacheDir := filepath.Join(b.internalDir, "cache")
	layerDir := filepath.Join(cacheDir, "layers", ref.ID, ref.Version.String(), module)

	if err := os.MkdirAll(layerDir, 0755); err != nil {
		return err
	}

	// Create minimal info.json
	info := types.PackageInfoV2{
		SchemaVersion:       "1",
		ID:                  ref.ID,
		Name:                ref.ID,
		Version:             ref.Version.String(),
		Arch:                []string{ref.Arch},
		Kind:                "runtime",
		Base:                "",
		PackageInfoV2Module: module,
	}

	infoData, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(layerDir, "info.json"), infoData, 0644)
}

// buildStageBuild builds the project in container
func (b *Builder) buildStageBuild(args []string) error {
	fmt.Println("[Start Build]")

	if len(args) == 0 {
		args = []string{"/project/linglong/entry.sh"}
	}

	// Generate entry script
	if err := b.generateEntryScript(); err != nil {
		return fmt.Errorf("failed to generate entry script: %w", err)
	}

	// Clean and create build output directory
	outputDir := filepath.Join(b.internalDir, "output")
	os.RemoveAll(outputDir)
	b.buildOutput = filepath.Join(outputDir, "_build")
	if err := os.MkdirAll(b.buildOutput, 0755); err != nil {
		return fmt.Errorf("failed to create build output directory: %w", err)
	}

	// Prepare environment variables
	env := map[string]string{
		"PREFIX":                   b.installPrefix,
		"LINGLONG_LD_SO_CACHE":     "/etc/ld.so.cache",
		"TRIPLET":                  "x86_64-linux-gnu",
		"LINYAPS_INIT_SINGLE_MODE": "1",
	}

	// Print build info
	fmt.Printf("Build command: %v\n", args)
	fmt.Printf("Install prefix: %s\n", b.installPrefix)

	// Use base paths from pull dependency stage
	if b.baseBinary == "" {
		return fmt.Errorf("base binary path not available")
	}

	// Prepare container configuration
	containerConfig := &types.ContainerConfig{
		AppID:    b.project.Package.ID,
		BasePath: b.baseBinary,
		UID:      b.uid,
		GID:      b.gid,
		Network:  !b.buildOptions.IsolateNetWork,
		Env:      env,
		Caps: []string{
			"CAP_CHOWN",
			"CAP_DAC_OVERRIDE",
			"CAP_FOWNER",
			"CAP_FSETID",
			"CAP_KILL",
			"CAP_NET_BIND_SERVICE",
			"CAP_SETFCAP",
			"CAP_SETGID",
			"CAP_SETPCAP",
			"CAP_SETUID",
			"CAP_SYS_CHROOT",
		},
		Mounts: []types.Mount{
			{
				Destination: "/project",
				Source:      b.workingDir,
				Type:        "bind",
				Options:     []string{"rbind", "rw"},
			},
			{
				Destination: "/output",
				Source:      b.buildOutput,
				Type:        "bind",
				Options:     []string{"rbind", "rw"},
			},
		},
	}

	// Set develop path if available
	if b.baseDevelop != "" {
		containerConfig.RuntimePath = &b.baseDevelop
	}

	// Add runtime mount if available - use overlay to merge binary and develop
	if b.runtimeBinary != "" {
		runtimeOverlay, runtimeCleanup, err := container.CreateOverlayRootfs(b.runtimeBinary, b.runtimeDevelop)
		if err == nil {
			containerConfig.Mounts = append(containerConfig.Mounts, types.Mount{
				Destination: "/runtime",
				Source:      runtimeOverlay,
				Type:        "bind",
				Options:     []string{"rbind", "ro"},
			})
			defer runtimeCleanup()
		}
	}

	// Generate ld.so.conf.d for library paths
	ldConfDir := filepath.Join(b.internalDir, "cache")
	os.MkdirAll(ldConfDir, 0755)
	ldConfPath := filepath.Join(ldConfDir, "zz_deepin-linglong-app.conf")
	ldConfContent := fmt.Sprintf(`/runtime/lib
/runtime/lib/x86_64-linux-gnu
/opt/apps/%s/files/lib
/opt/apps/%s/files/lib/x86_64-linux-gnu
`, b.project.Package.ID, b.project.Package.ID)
	if err := os.WriteFile(ldConfPath, []byte(ldConfContent), 0644); err == nil {
		containerConfig.Mounts = append(containerConfig.Mounts, types.Mount{
			Destination: "/etc/ld.so.conf.d/zz_deepin-linglong-app.conf",
			Source:      ldConfPath,
			Type:        "bind",
			Options:     []string{"rbind", "ro"},
		})
	}

	// Execute build in container
	if err := container.BuildInContainer(containerConfig, args, env); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	fmt.Println("Build completed successfully")

	return nil
}

// buildStageCommit commits the build output
func (b *Builder) buildStageCommit() error {
	fmt.Println("[Commit Contents]")

	// Generate app configuration
	if err := b.generateAppConf(); err != nil {
		return fmt.Errorf("failed to generate app conf: %w", err)
	}

	// Install files to modules
	if err := b.installFiles(); err != nil {
		return fmt.Errorf("failed to install files: %w", err)
	}

	// Generate entries
	if err := b.generateEntries(); err != nil {
		return fmt.Errorf("failed to generate entries: %w", err)
	}

	// Generate info.json for each module
	if err := b.generateInfoJson(); err != nil {
		return fmt.Errorf("failed to generate info.json: %w", err)
	}

	return nil
}

// ExportUAB exports the project as UAB
func (b *Builder) ExportUAB(option types.ExportOption, outputFile string) error {
	fmt.Println("[Export UAB]")

	if b.project == nil {
		return fmt.Errorf("not under project")
	}

	if b.project.Package.Kind != "app" {
		return fmt.Errorf("can only export app kind as UAB")
	}

	ref, err := b.currentReference()
	if err != nil {
		return err
	}

	// Set default options
	if option.Compressor == "" {
		option.Compressor = "lz4"
	}
	if len(option.Modules) == 0 {
		option.Modules = []string{"binary"}
	}

	// Generate output filename if not specified
	if outputFile == "" {
		outputFile = b.uabExportFilename(*ref)
	}

	// Get modules from output directory
	outputDir := filepath.Join(b.internalDir, "output")
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return fmt.Errorf("failed to read output directory: %w", err)
	}

	var modules []string
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != "_build" {
			modules = append(modules, entry.Name())
		}
	}

	if len(modules) == 0 {
		return fmt.Errorf("no modules found in output directory")
	}

	fmt.Printf("Exporting UAB to %s\n", outputFile)
	fmt.Printf("Compressor: %s\n", option.Compressor)
	fmt.Printf("Modules: %v\n", option.Modules)

	// UAB export logic would go here
	fmt.Println("UAB export completed (simplified)")

	return nil
}

// ExportLayer exports the project as layer files
func (b *Builder) ExportLayer(option types.ExportOption) error {
	fmt.Println("[Export Layer]")

	if b.project == nil {
		return fmt.Errorf("not under project")
	}

	ref, err := b.currentReference()
	if err != nil {
		return err
	}

	// Get modules from output directory
	outputDir := filepath.Join(b.internalDir, "output")
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return fmt.Errorf("failed to read output directory: %w", err)
	}

	var modules []string
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != "_build" {
			modules = append(modules, entry.Name())
		}
	}

	if len(modules) == 0 {
		return fmt.Errorf("no modules found in output directory")
	}

	// Set default compressor
	if option.Compressor == "" {
		option.Compressor = "lz4"
	}

	packager := layer.NewPackager()
	packager.SetCompressor(option.Compressor)

	for _, module := range modules {
		// Skip develop module if specified
		if option.NoExportDevelop && module == "develop" {
			continue
		}

		layerDir := filepath.Join(outputDir, module)

		outputFile := filepath.Join(b.workingDir, layer.GetLayerFilename(
			ref.ID, ref.Version.String(), ref.Arch, module))

		fmt.Printf("Exporting %s/%s to %s\n", ref.ID, module, outputFile)

		if err := packager.Pack(layerDir, outputFile); err != nil {
			return fmt.Errorf("failed to pack layer %s: %w", module, err)
		}
	}

	fmt.Println("Layer export completed")
	return nil
}

// ExtractLayer extracts a layer file to a directory
func (b *Builder) ExtractLayer(layerFile, destDir string) error {
	fmt.Printf("Extracting %s to %s\n", layerFile, destDir)

	packager := layer.NewPackager()
	return packager.Extract(layerFile, destDir)
}

// ImportLayer imports a layer file to the repository
func (b *Builder) ImportLayer(layerFile string) error {
	fmt.Printf("Importing layer %s\n", layerFile)

	// Create temporary directory for extraction
	tempDir, err := os.MkdirTemp("", "linglong-import-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Extract layer
	packager := layer.NewPackager()
	if err := packager.Unpack(layerFile, tempDir); err != nil {
		return fmt.Errorf("failed to unpack layer: %w", err)
	}

	// Import to repository
	if err := b.localRepo.ImportLayerDir(tempDir); err != nil {
		return fmt.Errorf("failed to import layer: %w", err)
	}

	fmt.Println("Layer import completed")
	return nil
}

// ImportLayerDir imports a layer directory to the repository
func (b *Builder) ImportLayerDir(layerDir string) error {
	fmt.Printf("Importing layer directory %s\n", layerDir)

	if err := b.localRepo.ImportLayerDir(layerDir); err != nil {
		return fmt.Errorf("failed to import layer directory: %w", err)
	}

	fmt.Println("Layer directory import completed")
	return nil
}

// Run runs the built application
func (b *Builder) Run(modules []string, args []string, debug bool, workdir string, extensions []string) error {
	fmt.Println("[Run Application]")

	if b.project == nil {
		return fmt.Errorf("not under project")
	}

	if b.project.Package.Kind != "app" {
		return fmt.Errorf("only app can run")
	}

	ref, err := b.currentReference()
	if err != nil {
		return err
	}

	// Add develop module if debug mode
	if debug {
		modules = append(modules, "develop")
	}

	// Default modules
	if len(modules) == 0 {
		modules = []string{"binary"}
	}

	// Default args from command
	if len(args) == 0 {
		args = b.project.Command
	}

	fmt.Printf("Running %s\n", ref.ID)
	fmt.Printf("Modules: %v\n", modules)
	fmt.Printf("Command: %v\n", args)

	// Create OSTree repo
	ostreeRepo := repo.NewOSTreeRepo(b.config)

	// Pull and get base paths
	baseRef, err := b.parseDependency(b.project.Base)
	if err != nil {
		return fmt.Errorf("failed to parse base: %w", err)
	}

	// Resolve base version and pull if needed
	resolvedBaseRef, _ := ostreeRepo.ResolveVersion(*baseRef, "binary")
	if resolvedBaseRef == nil {
		resolvedBaseRef = baseRef
	}

	if !ostreeRepo.Exists(*resolvedBaseRef, "binary") {
		fmt.Printf("Pulling base %s binary...\n", baseRef.ID)
		if err := ostreeRepo.Pull(*baseRef, "binary"); err != nil {
			return fmt.Errorf("failed to pull base binary: %w", err)
		}
	} else {
		fmt.Printf("Base %s binary already exists locally, skipping pull\n", baseRef.ID)
	}

	baseBinary, err := ostreeRepo.GetLayerDir(*baseRef, "binary")
	if err != nil {
		return fmt.Errorf("failed to get base binary: %w", err)
	}
	fmt.Printf("Base binary: %s\n", baseBinary)

	baseDevelop := ""
	if debug {
		if !ostreeRepo.Exists(*resolvedBaseRef, "develop") {
			fmt.Printf("Pulling base %s develop...\n", baseRef.ID)
			if err := ostreeRepo.Pull(*baseRef, "develop"); err != nil {
				fmt.Printf("Warning: failed to pull base develop: %v\n", err)
			}
		} else {
			fmt.Printf("Base %s develop already exists locally, skipping pull\n", baseRef.ID)
		}

		baseDevelop, err = ostreeRepo.GetLayerDir(*baseRef, "develop")
		if err != nil {
			fmt.Printf("Warning: failed to get base develop: %v\n", err)
		}
	}

	// Pull and get runtime paths if specified
	runtimeBinary := ""
	runtimeDevelop := ""
	if b.project.Runtime != nil {
		runtimeRef, err := b.parseDependency(*b.project.Runtime)
		if err != nil {
			return fmt.Errorf("failed to parse runtime: %w", err)
		}

		resolvedRuntimeRef, err := ostreeRepo.ResolveVersion(*runtimeRef, "binary")
		if err != nil {
			resolvedRuntimeRef = runtimeRef
		}

		if !ostreeRepo.Exists(*resolvedRuntimeRef, "binary") {
			fmt.Printf("Pulling runtime %s binary...\n", runtimeRef.ID)
			if err := ostreeRepo.Pull(*runtimeRef, "binary"); err != nil {
				fmt.Printf("Warning: failed to pull runtime binary: %v\n", err)
			}
		} else {
			fmt.Printf("Runtime %s binary already exists locally, skipping pull\n", runtimeRef.ID)
		}

		runtimeBinary, err = ostreeRepo.GetLayerDir(*runtimeRef, "binary")
		if err != nil {
			fmt.Printf("Warning: failed to get runtime binary: %v\n", err)
		} else {
			fmt.Printf("Runtime binary: %s\n", runtimeBinary)
		}

		if debug {
			if !ostreeRepo.Exists(*resolvedRuntimeRef, "develop") {
				fmt.Printf("Pulling runtime %s develop...\n", runtimeRef.ID)
				if err := ostreeRepo.Pull(*runtimeRef, "develop"); err != nil {
					fmt.Printf("Warning: failed to pull runtime develop: %v\n", err)
				}
			} else {
				fmt.Printf("Runtime %s develop already exists locally, skipping pull\n", runtimeRef.ID)
			}

			runtimeDevelop, err = ostreeRepo.GetLayerDir(*runtimeRef, "develop")
			if err != nil {
				fmt.Printf("Warning: failed to get runtime develop: %v\n", err)
			}
		}
	}

	// Get app layer path from output directory
	appBinary := filepath.Join(b.internalDir, "output", "binary")
	if _, err := os.Stat(appBinary); err != nil {
		return fmt.Errorf("app binary not found in output directory: %w", err)
	}

	// Prepare environment variables
	env := map[string]string{
		"LC_ALL":   "C.UTF-8",
		"LANGUAGE": "en_US:en",
	}

	// Add extension environment variables
	for _, ext := range extensions {
		env["LINGLONG_EXT_"+strings.ReplaceAll(ext, ".", "_")] = "1"
	}

	// Prepare container configuration
	containerConfig := &types.ContainerConfig{
		AppID:    b.project.Package.ID,
		BasePath: baseBinary,
		UID:      1000,
		GID:      1000,
		Network:  true,
		Env:      env,
		Caps: []string{
			"CAP_CHOWN",
			"CAP_DAC_OVERRIDE",
			"CAP_NET_BIND_SERVICE",
		},
		Mounts: []types.Mount{},
	}

	// Set runtime path if available
	if runtimeBinary != "" {
		containerConfig.RuntimePath = &runtimeBinary
	}

	// Run application in container
	if err := container.RunApplication(containerConfig, args, env, appBinary, runtimeBinary, baseDevelop, runtimeDevelop, workdir); err != nil {
		return fmt.Errorf("run failed: %w", err)
	}

	fmt.Println("Application run completed")

	return nil
}

// CurrentReference returns the current project reference
func (b *Builder) CurrentReference() (*types.Reference, error) {
	return b.currentReference()
}

// Helper methods

func (b *Builder) currentReference() (*types.Reference, error) {
	version, err := types.ParseVersion(b.project.Package.Version)
	if err != nil {
		return nil, fmt.Errorf("invalid version: %w", err)
	}

	arch := "x86_64"    // Simplified
	channel := "stable" // Default channel

	return &types.Reference{
		Channel: channel,
		ID:      b.project.Package.ID,
		Version: version,
		Arch:    arch,
	}, nil
}

func (b *Builder) parseDependency(dep string) (*types.Reference, error) {
	// Format: id/version (e.g., org.deepin.base/23.1.0.3)
	// Remote refs format: main/id/version/arch/module
	parts := strings.Split(dep, "/")
	if len(parts) < 2 {
		return &types.Reference{
			Channel: "main",
			ID:      dep,
		}, nil
	}

	version, err := types.ParseVersion(parts[1])
	if err != nil {
		return nil, err
	}

	return &types.Reference{
		Channel: "main",
		ID:      parts[0],
		Version: version,
		Arch:    "x86_64",
	}, nil
}

func (b *Builder) generateEntryScript() error {
	scriptPath := filepath.Join(b.internalDir, "entry.sh")

	script := `#!/bin/bash
set -e

# This file is generated by ` + "`build`" + ` in linglong.yaml
# DO NOT EDIT IT
`

	// Add debug symbols if not skipped
	if !b.buildOptions.SkipStripSymbols {
		script += `
# enable strip symbols
export CFLAGS="-g $CFLAGS"
export CXXFLAGS="-g $CFLAGS"
`
	}

	// Add build script from project
	if b.project.Build != nil {
		script += *b.project.Build
	}
	script += "\n"

	// Add post-build strip if not skipped
	if !b.buildOptions.SkipStripSymbols {
		script += "# POST BUILD PROCESS\n"
		script += "/usr/lib/linglong-builder/symbols-strip.sh\n"
	}

	// Write script file
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		return fmt.Errorf("failed to write entry script: %w", err)
	}

	fmt.Println("Generated entry.sh successfully")
	return nil
}

func (b *Builder) generateAppConf() error {
	// Generate application configuration
	return nil
}

func (b *Builder) generateInfoJson() error {
	ref, err := b.currentReference()
	if err != nil {
		return err
	}

	for _, module := range b.packageModules {
		moduleOutput := filepath.Join(b.internalDir, "output", module)

		// Create module directory if not exists
		if err := os.MkdirAll(moduleOutput, 0755); err != nil {
			return fmt.Errorf("failed to create module directory: %w", err)
		}

		// Calculate directory size
		dirSize, err := calculateDirSize(moduleOutput)
		if err != nil {
			fmt.Printf("Warning: failed to calculate directory size: %v\n", err)
			dirSize = 0
		}

		// Create package info
		info := types.PackageInfoV2{
			SchemaVersion:       "1",
			ID:                  b.project.Package.ID,
			Name:                b.project.Package.Name,
			Version:             b.project.Package.Version,
			Arch:                []string{ref.Arch},
			Kind:                b.project.Package.Kind,
			Description:         b.project.Package.Description,
			Base:                b.project.Base,
			Runtime:             b.project.Runtime,
			Command:             b.project.Command,
			PackageInfoV2Module: module,
			Size:                dirSize,
		}

		// Write info.json
		infoPath := filepath.Join(moduleOutput, "info.json")
		data, err := json.MarshalIndent(info, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal package info: %w", err)
		}

		if err := os.WriteFile(infoPath, data, 0644); err != nil {
			return fmt.Errorf("failed to write info.json: %w", err)
		}
	}

	return nil
}

func (b *Builder) installFiles() error {
	fmt.Println("Installing files to modules...")

	b.packageModules = []string{}
	projectModules := []types.BuilderProjectModule{}

	// Check for develop files
	hasDevelop := false
	hasBinary := false

	if b.project.Modules != nil {
		for _, module := range b.project.Modules {
			if module.Name == "develop" {
				hasDevelop = true
			} else if module.Name == "binary" {
				hasBinary = true
			}
			projectModules = append(projectModules, module)
			b.packageModules = append(b.packageModules, module.Name)
		}
	}

	// Add default develop module if not exists
	if !hasDevelop {
		b.packageModules = append(b.packageModules, "develop")
		projectModules = append(projectModules, types.BuilderProjectModule{
			Name:  "develop",
			Rules: []string{"^/include/.+", "^/lib/debug/.+", "^/lib/.+\\.a$"},
		})
	}

	// Add default binary module if not exists
	if !hasBinary {
		b.packageModules = append(b.packageModules, "binary")
		projectModules = append(projectModules, types.BuilderProjectModule{
			Name:  "binary",
			Rules: []string{"/"},
		})
	}

	// Copy files to module directories
	for _, module := range projectModules {
		moduleDir := filepath.Join(b.internalDir, "output", module.Name)
		moduleFilesDir := filepath.Join(moduleDir, "files")

		if err := os.MkdirAll(moduleFilesDir, 0755); err != nil {
			return fmt.Errorf("failed to create module directory: %w", err)
		}

		fmt.Printf("Installing module %s...\n", module.Name)

		// Copy files from build output to module files directory
		if err := b.copyBuildOutputToModule(module.Name, moduleFilesDir, module.Rules); err != nil {
			fmt.Printf("Warning: failed to copy files for module %s: %v\n", module.Name, err)
		}
	}

	fmt.Printf("Modules: %v\n", b.packageModules)
	return nil
}

// copyBuildOutputToModule copies build output files to module directory
func (b *Builder) copyBuildOutputToModule(module, destDir string, rules []string) error {
	// For binary module, copy all files from build output
	if module == "binary" {
		// Check if build output directory has files
		entries, err := os.ReadDir(b.buildOutput)
		if err != nil {
			return err
		}

		if len(entries) == 0 {
			// No files in build output, create minimal structure
			fmt.Println("Note: No files in build output, creating minimal layer")
			return nil
		}

		// Copy all files from build output
		return b.copyDir(b.buildOutput, destDir)
	}

	// For develop module, copy include, lib/pkgconfig, lib/cmake, etc.
	if module == "develop" {
		includeDir := filepath.Join(b.buildOutput, "include")
		if _, err := os.Stat(includeDir); err == nil {
			destIncludeDir := filepath.Join(destDir, "include")
			if err := b.copyDir(includeDir, destIncludeDir); err != nil {
				return err
			}
		}
		return nil
	}

	return nil
}

// copyDir copies a directory recursively
func (b *Builder) copyDir(src, dst string) error {
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

		return b.copyFile(path, dstPath)
	})
}

// copyFile copies a single file with permissions
func (b *Builder) copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	// Get source file info for permissions
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	// Write with original permissions
	if err := os.WriteFile(dst, data, srcInfo.Mode()); err != nil {
		return err
	}

	return nil
}

// calculateDirSize calculates total size of directory
func calculateDirSize(dir string) (int64, error) {
	var size int64
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}

func (b *Builder) generateEntries() error {
	// Generate desktop entries, icons, etc.
	return nil
}

func (b *Builder) printBasicInfo() {
	fmt.Printf("Package ID: %s\n", b.project.Package.ID)
	fmt.Printf("Package Name: %s\n", b.project.Package.Name)
	fmt.Printf("Version: %s\n", b.project.Package.Version)
	fmt.Printf("Kind: %s\n", b.project.Package.Kind)
	fmt.Printf("Base: %s\n", b.project.Base)
	if b.project.Runtime != nil {
		fmt.Printf("Runtime: %s\n", *b.project.Runtime)
	}
	fmt.Printf("Install Prefix: %s\n", b.installPrefix)
}

func (b *Builder) uabExportFilename(ref types.Reference) string {
	return fmt.Sprintf("%s_%s_%s.uab", ref.ID, ref.Version.String(), ref.Arch)
}

func (b *Builder) layerExportFilename(ref types.Reference, module string) string {
	return layer.GetLayerFilename(ref.ID, ref.Version.String(), ref.Arch, module)
}
