package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/OpenAtom-Linyaps/linyaps/golang/ll-builder/internal/builder"
	"github.com/OpenAtom-Linyaps/linyaps/golang/ll-builder/internal/config"
	"github.com/OpenAtom-Linyaps/linyaps/golang/ll-builder/internal/repo"
)

var (
	runModules    []string
	runDebug      bool
	runWorkdir    string
	runExtensions []string
)

var runCmd = &cobra.Command{
	Use:   "run [COMMAND...]",
	Short: "Run built Linyaps app",
	Long:  "Run a built Linyaps application for testing. Supports debug mode and custom working directory.",
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

		// Run application
		return b.Run(runModules, args, runDebug, runWorkdir, runExtensions)
	},
}

func init() {
	runCmd.Flags().StringVarP(&filePath, "file", "f", "", "Path to the linglong.yaml file")
	runCmd.Flags().StringSliceVar(&runModules, "modules", nil, "Run specified modules (e.g., binary,develop)")
	runCmd.Flags().BoolVar(&runDebug, "debug", false, "Run in debug mode (enable develop module)")
	runCmd.Flags().StringVar(&runWorkdir, "workdir", "", "Specify working directory")
	runCmd.Flags().StringSliceVar(&runExtensions, "extensions", nil, "Specify extensions to use")

	rootCmd.AddCommand(runCmd)
}
