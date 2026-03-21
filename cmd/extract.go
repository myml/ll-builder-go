package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/OpenAtom-Linyaps/linyaps/golang/ll-builder/internal/builder"
	"github.com/OpenAtom-Linyaps/linyaps/golang/ll-builder/internal/config"
	"github.com/OpenAtom-Linyaps/linyaps/golang/ll-builder/internal/repo"
)

var extractCmd = &cobra.Command{
	Use:   "extract LAYER DIR",
	Short: "Extract Linyaps layer to dir",
	Long:  "Extract a Linyaps layer file to a directory.",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		layerFile := args[0]
		destDir := args[1]

		// Load builder config
		cfg, err := config.LoadConfig()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		// Create repository
		ostreeRepo := repo.NewLocalOSTreeRepo("", cfg)

		// Create builder
		b := builder.NewBuilder(nil, ".", ostreeRepo, cfg)

		// Extract layer
		return b.ExtractLayer(layerFile, destDir)
	},
}

func init() {
	rootCmd.AddCommand(extractCmd)
}
