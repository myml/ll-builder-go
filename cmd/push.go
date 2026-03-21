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
	pushRepoURL  string
	pushRepoName string
	pushModule   string
)

var pushCmd = &cobra.Command{
	Use:   "push",
	Short: "Push Linyaps app to remote repo",
	Long:  "Push built Linyaps application to a remote repository.",
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
		ostreeRepo := repo.NewLocalOSTreeRepo("", cfg)

		// Create builder
		b := builder.NewBuilder(project, filepath.Dir(yamlPath), ostreeRepo, cfg)

		// Push application
		if pushModule != "" {
			return b.Push(pushModule, pushRepoURL, pushRepoName)
		}

		// Push all modules
		ref, err := b.CurrentReference()
		if err != nil {
			return err
		}
		modules := ostreeRepo.GetModuleList(*ref)
		for _, module := range modules {
			if err := b.Push(module, pushRepoURL, pushRepoName); err != nil {
				return err
			}
		}

		return nil
	},
}

func init() {
	pushCmd.Flags().StringVarP(&filePath, "file", "f", "", "Path to the linglong.yaml file")
	pushCmd.Flags().StringVar(&pushRepoURL, "repo-url", "", "Remote repository URL")
	pushCmd.Flags().StringVar(&pushRepoName, "repo-name", "", "Remote repository name")
	pushCmd.Flags().StringVar(&pushModule, "module", "", "Push single module")

	rootCmd.AddCommand(pushCmd)
}
