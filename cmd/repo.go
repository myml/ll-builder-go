package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/OpenAtom-Linyaps/linyaps/golang/ll-builder/internal/config"
	"github.com/OpenAtom-Linyaps/linyaps/golang/ll-builder/internal/repo"
	"github.com/OpenAtom-Linyaps/linyaps/golang/ll-builder/internal/types"
)

var repoCmd = &cobra.Command{
	Use:   "repo",
	Short: "Display and manage repositories",
	Long:  "Display and manage Linyaps repositories.",
}

var repoShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show repository information",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadConfig()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		ostreeRepo := repo.NewOSTreeRepo(cfg)
		repoConfig := ostreeRepo.GetConfig()

		fmt.Printf("Default: %s\n", repoConfig.DefaultRepo)
		fmt.Printf("%-15s %-50s %-15s\n", "Name", "Url", "Alias")

		for _, r := range repoConfig.Repos {
			alias := r.Name
			if r.Alias != nil {
				alias = *r.Alias
			}
			fmt.Printf("%-15s %-50s %-15s\n", r.Name, r.URL, alias)
		}

		return nil
	},
}

var (
	repoAddName  string
	repoAddURL   string
	repoAddAlias string
)

var repoAddCmd = &cobra.Command{
	Use:   "add NAME URL",
	Short: "Add a new repository",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		url := args[1]

		cfg, err := config.LoadConfig()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		ostreeRepo := repo.NewOSTreeRepo(cfg)
		repoConfig := ostreeRepo.GetConfig()

		// Check if repo already exists
		for _, r := range repoConfig.Repos {
			if r.Name == name {
				return fmt.Errorf("repo %s already exists", name)
			}
		}

		// Add new repo
		newRepo := types.RepoConfig{
			Name: name,
			URL:  url,
		}
		if repoAddAlias != "" {
			newRepo.Alias = &repoAddAlias
		}

		repoConfig.Repos = append(repoConfig.Repos, newRepo)

		if err := ostreeRepo.SetConfig(repoConfig); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		fmt.Printf("Repository %s added successfully\n", name)
		return nil
	},
}

var repoRemoveCmd = &cobra.Command{
	Use:   "remove NAME",
	Short: "Remove a repository",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		cfg, err := config.LoadConfig()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		ostreeRepo := repo.NewOSTreeRepo(cfg)
		repoConfig := ostreeRepo.GetConfig()

		// Check if it's the default repo
		if repoConfig.DefaultRepo == name {
			return fmt.Errorf("cannot remove default repo, change default repo first")
		}

		// Find and remove repo
		found := false
		newRepos := []types.RepoConfig{}
		for _, r := range repoConfig.Repos {
			if r.Name == name {
				found = true
				continue
			}
			newRepos = append(newRepos, r)
		}

		if !found {
			return fmt.Errorf("repo %s not found", name)
		}

		repoConfig.Repos = newRepos

		if err := ostreeRepo.SetConfig(repoConfig); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		fmt.Printf("Repository %s removed successfully\n", name)
		return nil
	},
}

var repoUpdateCmd = &cobra.Command{
	Use:   "update NAME URL",
	Short: "Update repository URL",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		url := args[1]

		cfg, err := config.LoadConfig()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		ostreeRepo := repo.NewOSTreeRepo(cfg)
		repoConfig := ostreeRepo.GetConfig()

		// Find and update repo
		found := false
		for i, r := range repoConfig.Repos {
			if r.Name == name {
				repoConfig.Repos[i].URL = url
				found = true
				break
			}
		}

		if !found {
			return fmt.Errorf("repo %s not found", name)
		}

		if err := ostreeRepo.SetConfig(repoConfig); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		fmt.Printf("Repository %s updated successfully\n", name)
		return nil
	},
}

var repoSetDefaultCmd = &cobra.Command{
	Use:   "set-default NAME",
	Short: "Set default repository",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		cfg, err := config.LoadConfig()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		ostreeRepo := repo.NewOSTreeRepo(cfg)
		repoConfig := ostreeRepo.GetConfig()

		// Check if repo exists
		found := false
		for _, r := range repoConfig.Repos {
			if r.Name == name {
				found = true
				break
			}
		}

		if !found {
			return fmt.Errorf("repo %s not found", name)
		}

		repoConfig.DefaultRepo = name

		if err := ostreeRepo.SetConfig(repoConfig); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		fmt.Printf("Default repository set to %s\n", name)
		return nil
	},
}

var repoEnableMirrorCmd = &cobra.Command{
	Use:   "enable-mirror NAME",
	Short: "Enable mirror for repository",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		cfg, err := config.LoadConfig()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		ostreeRepo := repo.NewOSTreeRepo(cfg)
		repoConfig := ostreeRepo.GetConfig()

		// Find and enable mirror
		found := false
		for i, r := range repoConfig.Repos {
			if r.Name == name {
				enabled := true
				repoConfig.Repos[i].MirrorEnabled = &enabled
				found = true
				break
			}
		}

		if !found {
			return fmt.Errorf("repo %s not found", name)
		}

		if err := ostreeRepo.SetConfig(repoConfig); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		fmt.Printf("Mirror enabled for repository %s\n", name)
		return nil
	},
}

var repoDisableMirrorCmd = &cobra.Command{
	Use:   "disable-mirror NAME",
	Short: "Disable mirror for repository",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		cfg, err := config.LoadConfig()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		ostreeRepo := repo.NewOSTreeRepo(cfg)
		repoConfig := ostreeRepo.GetConfig()

		// Find and disable mirror
		found := false
		for i, r := range repoConfig.Repos {
			if r.Name == name {
				enabled := false
				repoConfig.Repos[i].MirrorEnabled = &enabled
				found = true
				break
			}
		}

		if !found {
			return fmt.Errorf("repo %s not found", name)
		}

		if err := ostreeRepo.SetConfig(repoConfig); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		fmt.Printf("Mirror disabled for repository %s\n", name)
		return nil
	},
}

func init() {
	repoAddCmd.Flags().StringVar(&repoAddAlias, "alias", "", "Repository alias")

	repoCmd.AddCommand(repoShowCmd)
	repoCmd.AddCommand(repoAddCmd)
	repoCmd.AddCommand(repoRemoveCmd)
	repoCmd.AddCommand(repoUpdateCmd)
	repoCmd.AddCommand(repoSetDefaultCmd)
	repoCmd.AddCommand(repoEnableMirrorCmd)
	repoCmd.AddCommand(repoDisableMirrorCmd)

	rootCmd.AddCommand(repoCmd)
}
