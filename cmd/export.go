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
	exportLayer      bool
	exportCompressor string
	exportIcon       string
	exportLoader     string
	exportNoDevelop  bool
	exportOutput     string
	exportRef        string
	exportModules    []string
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export to Linyaps layer or UAB",
	Long:  "Export built application as Linyaps layer files or UAB format. UAB is the recommended format.",
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

		// Create export options
		exportOpts := types.ExportOption{
			Compressor:      exportCompressor,
			IconPath:        exportIcon,
			Loader:          exportLoader,
			Ref:             exportRef,
			Modules:         exportModules,
			NoExportDevelop: exportNoDevelop,
		}

		// Export
		if exportLayer {
			return b.ExportLayer(exportOpts)
		}
		return b.ExportUAB(exportOpts, exportOutput)
	},
}

func init() {
	exportCmd.Flags().StringVarP(&filePath, "file", "f", "", "Path to the linglong.yaml file")
	exportCmd.Flags().StringVarP(&exportCompressor, "compressor", "z", "", "Compressor: lz4, lzma, zstd (default: lz4)")
	exportCmd.Flags().StringVar(&exportIcon, "icon", "", "UAB icon file")
	exportCmd.Flags().StringVar(&exportLoader, "loader", "", "Custom loader file")
	exportCmd.Flags().BoolVar(&exportLayer, "layer", false, "Export as layer file (deprecated)")
	exportCmd.Flags().BoolVar(&exportNoDevelop, "no-develop", false, "Don't export develop module")
	exportCmd.Flags().StringVarP(&exportOutput, "output", "o", "", "Output file")
	exportCmd.Flags().StringVar(&exportRef, "ref", "", "Package reference")
	exportCmd.Flags().StringSliceVar(&exportModules, "modules", nil, "Modules to export")

	rootCmd.AddCommand(exportCmd)
}
