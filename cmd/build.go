package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/OpenAtom-Linyaps/linyaps/golang/ll-builder/internal/builder"
	"github.com/OpenAtom-Linyaps/linyaps/golang/ll-builder/internal/config"
	"github.com/OpenAtom-Linyaps/linyaps/golang/ll-builder/internal/repo"
	"github.com/OpenAtom-Linyaps/linyaps/golang/ll-builder/internal/types"
)

var (
	buildOffline      bool
	skipFetchSource   bool
	skipPullDepend    bool
	skipRunContainer  bool
	skipCommitOutput  bool
	skipOutputCheck   bool
	skipStripSymbols  bool
	isolateNetwork    bool
	fullDevelopModule bool
)

var buildCmd = &cobra.Command{
	Use:   "build [COMMAND...]",
	Short: "Build Linyaps project",
	Long:  "Build a Linyaps application. After the build is complete, the build content will be automatically committed to the local ostree cache.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}

		// Get project YAML path
		yamlPath, err := config.GetProjectYAMLPath(cwd, filePath)
		if err != nil {
			return fmt.Errorf("failed to find project file: %w", err)
		}

		// Load project
		project, err := config.LoadProject(yamlPath)
		if err != nil {
			return fmt.Errorf("failed to load project: %w", err)
		}

		// Load builder config
		cfg, err := config.LoadConfig()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		// Create repository
		ostreeRepo := repo.NewOSTreeRepo(cfg)

		// Create builder
		b := builder.NewBuilder(project, filepath.Dir(yamlPath), ostreeRepo, cfg)

		// Set build options
		buildOpts := types.BuilderBuildOptions{
			FullDevelop:      fullDevelopModule,
			SkipFetchSource:  skipFetchSource || buildOffline,
			SkipPullDepend:   skipPullDepend || buildOffline,
			SkipRunContainer: skipRunContainer,
			SkipCommitOutput: skipCommitOutput,
			SkipCheckOutput:  skipOutputCheck,
			SkipStripSymbols: true, // Default skip strip since script may not exist
			IsolateNetWork:   isolateNetwork,
		}
		b.SetBuildOptions(buildOpts)

		// Build
		return b.Build(args)
	},
}

func init() {
	buildCmd.Flags().StringVarP(&filePath, "file", "f", "", "Path to the linglong.yaml file")
	buildCmd.Flags().BoolVar(&buildOffline, "offline", false, "Use only local files")
	buildCmd.Flags().BoolVar(&skipFetchSource, "skip-fetch-source", false, "Skip fetching source code")
	buildCmd.Flags().BoolVar(&skipPullDepend, "skip-pull-depend", false, "Skip pulling dependencies")
	buildCmd.Flags().BoolVar(&skipRunContainer, "skip-run-container", false, "Skip running container")
	buildCmd.Flags().BoolVar(&skipCommitOutput, "skip-commit-output", false, "Skip committing build output")
	buildCmd.Flags().BoolVar(&skipOutputCheck, "skip-output-check", false, "Skip output checking")
	buildCmd.Flags().BoolVar(&skipStripSymbols, "skip-strip-symbols", false, "Skip stripping debug symbols")
	buildCmd.Flags().BoolVar(&isolateNetwork, "isolate-network", false, "Build in isolated network environment")
	buildCmd.Flags().BoolVar(&fullDevelopModule, "full-develop-module", false, "Build full development packages")

	rootCmd.AddCommand(buildCmd)
}
