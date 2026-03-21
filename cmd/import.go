package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/OpenAtom-Linyaps/linyaps/golang/ll-builder/internal/builder"
	"github.com/OpenAtom-Linyaps/linyaps/golang/ll-builder/internal/config"
	"github.com/OpenAtom-Linyaps/linyaps/golang/ll-builder/internal/repo"
)

var importCmd = &cobra.Command{
	Use:   "import LAYER",
	Short: "Import Linyaps layer to build repo",
	Long:  "Import a Linyaps layer file into the local build repository.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		layerFile := args[0]

		// Load builder config
		cfg, err := config.LoadConfig()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		// Create repository
		ostreeRepo := repo.NewOSTreeRepo(cfg)

		// Create builder
		b := builder.NewBuilder(nil, ".", ostreeRepo, cfg)

		// Import layer
		return b.ImportLayer(layerFile)
	},
}

var importDirCmd = &cobra.Command{
	Use:   "import-dir PATH",
	Short: "Import Linyaps layer dir to build repo",
	Long:  "Import a Linyaps layer directory into the local build repository.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		layerDir := args[0]

		// Load builder config
		cfg, err := config.LoadConfig()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		// Create repository
		ostreeRepo := repo.NewOSTreeRepo(cfg)

		// Create builder
		b := builder.NewBuilder(nil, ".", ostreeRepo, cfg)

		// Import layer directory
		return b.ImportLayerDir(layerDir)
	},
}

func init() {
	rootCmd.AddCommand(importCmd)
	rootCmd.AddCommand(importDirCmd)
}
