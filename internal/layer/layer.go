package layer

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/klauspost/compress/zstd"
	"github.com/klauspost/pgzip"
	"github.com/pierrec/lz4/v4"
)

const (
	// Magic number for layer files: "<<< deepin linglong layer archive >>>" padded to 40 bytes
	layerMagicNumber = "<<< deepin linglong layer archive >>>"
	layerMagicLength = 40
)

// Packager handles layer file packaging
type Packager struct {
	compressor string
	workDir    string
}

// NewPackager creates a new layer packager
func NewPackager() *Packager {
	return &Packager{
		compressor: "lz4",
	}
}

// SetCompressor sets the compressor type
func (p *Packager) SetCompressor(compressor string) {
	p.compressor = compressor
}

// Pack packs a layer directory into a layer file with proper magic number
// Uses erofs filesystem format to match C++ version
func (p *Packager) Pack(layerDir, outputFile string) error {
	// Read info.json from layer directory
	infoPath := filepath.Join(layerDir, "info.json")
	infoData, err := os.ReadFile(infoPath)
	if err != nil {
		return fmt.Errorf("failed to read info.json: %w", err)
	}

	// Parse info and convert to old format
	var info map[string]interface{}
	if err := json.Unmarshal(infoData, &info); err != nil {
		return fmt.Errorf("failed to parse info.json: %w", err)
	}

	// Convert to old PackageInfo format (V1) - use "module" as JSON field name
	oldInfo := map[string]interface{}{
		"appid":       info["id"],
		"arch":        info["arch"],
		"base":        info["base"],
		"channel":     info["channel"],
		"command":     info["command"],
		"description": info["description"],
		"kind":        info["kind"],
		"module":      info["packageInfoV2Module"],
		"name":        info["name"],
		"runtime":     info["runtime"],
		"size":        0,
		"version":     info["version"],
	}

	// Set size if available
	if size, ok := info["size"]; ok && size != nil {
		oldInfo["size"] = size
	}

	// Create LayerInfo structure
	layerInfo := map[string]interface{}{
		"version": "1",
		"info":    oldInfo,
	}

	layerInfoJSON, err := json.Marshal(layerInfo)
	if err != nil {
		return fmt.Errorf("failed to marshal layer info: %w", err)
	}

	// Create temporary work directory with converted info.json
	tmpDir, err := os.MkdirTemp("", "linglong-layer-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Copy layer directory to temp dir
	tmpLayerDir := filepath.Join(tmpDir, "layer")
	if err := copyDir(layerDir, tmpLayerDir); err != nil {
		return fmt.Errorf("failed to copy layer dir: %w", err)
	}

	// Write old format info.json to temp dir (for erofs contents)
	oldInfoJSON, err := json.MarshalIndent(oldInfo, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal old info: %w", err)
	}
	if err := os.WriteFile(filepath.Join(tmpLayerDir, "info.json"), oldInfoJSON, 0644); err != nil {
		return fmt.Errorf("failed to write old info.json: %w", err)
	}

	// Create erofs image from temp directory
	erofsFile := filepath.Join(tmpDir, "layer.erofs")
	if err := p.createErofs(tmpLayerDir, erofsFile); err != nil {
		return fmt.Errorf("failed to create erofs: %w", err)
	}

	// Read erofs data
	erofsData, err := os.ReadFile(erofsFile)
	if err != nil {
		return fmt.Errorf("failed to read erofs file: %w", err)
	}

	// Create output buffer
	// Format: magic(40) + metaInfoLen(4) + metaInfo + erofs
	metaInfoLen := uint32(len(layerInfoJSON))
	magic := make([]byte, layerMagicLength)
	copy(magic, layerMagicNumber)

	output := make([]byte, layerMagicLength+4+len(layerInfoJSON)+len(erofsData))
	copy(output, magic)
	binary.LittleEndian.PutUint32(output[layerMagicLength:layerMagicLength+4], metaInfoLen)
	copy(output[layerMagicLength+4:layerMagicLength+4+len(layerInfoJSON)], layerInfoJSON)
	copy(output[layerMagicLength+4+len(layerInfoJSON):], erofsData)

	// Write entire layer file at once
	if err := os.WriteFile(outputFile, output, 0644); err != nil {
		return fmt.Errorf("failed to write layer file: %w", err)
	}

	return nil
}

// createErofs creates an erofs filesystem image
func (p *Packager) createErofs(srcDir, outputFile string) error {
	compressor := p.compressor
	if compressor == "" {
		compressor = "lz4"
	}

	// Check for mkfs.erofs
	mkfsErofs, err := exec.LookPath("mkfs.erofs")
	if err != nil {
		// Fallback: use tar compression if mkfs.erofs not available
		return p.createTarFallback(srcDir, outputFile)
	}

	cmd := exec.Command(mkfsErofs,
		"-z"+compressor,
		"-b4096",
		outputFile,
		"--exclude-regex=minified*",
		srcDir,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		// Fallback to tar
		return p.createTarFallback(srcDir, outputFile)
	}

	return nil
}

// createTarFallback creates a tar archive as fallback when erofs is not available
func (p *Packager) createTarFallback(srcDir, outputFile string) error {
	// Use tar command
	cmd := exec.Command("tar", "-cf", outputFile, "-C", srcDir, ".")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tar failed: %w", err)
	}
	return nil
}

// Unpack unpacks a layer file to a directory
func (p *Packager) Unpack(layerFile, destDir string) error {
	// Open layer file
	file, err := os.Open(layerFile)
	if err != nil {
		return fmt.Errorf("failed to open layer file: %w", err)
	}
	defer file.Close()

	// Read and verify magic number
	magic := make([]byte, layerMagicLength)
	if _, err := io.ReadFull(file, magic); err != nil {
		return fmt.Errorf("failed to read magic number: %w", err)
	}

	// Check magic number
	magicStr := string(magic[:len(layerMagicNumber)])
	if magicStr != layerMagicNumber {
		// Try legacy format (just compressed tar)
		file.Seek(0, 0)
		return p.unpackLegacy(file, destDir)
	}

	// Read meta info length
	var metaInfoLen uint32
	if err := binary.Read(file, binary.LittleEndian, &metaInfoLen); err != nil {
		return fmt.Errorf("failed to read meta info length: %w", err)
	}

	// Read meta info
	metaInfo := make([]byte, metaInfoLen)
	if _, err := io.ReadFull(file, metaInfo); err != nil {
		return fmt.Errorf("failed to read meta info: %w", err)
	}

	// Save meta info as info.json
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create dest dir: %w", err)
	}

	// Parse layer info
	var layerInfo map[string]interface{}
	if err := json.Unmarshal(metaInfo, &layerInfo); err != nil {
		return fmt.Errorf("failed to parse layer info: %w", err)
	}

	// Get info from layer info
	if info, ok := layerInfo["info"].(map[string]interface{}); ok {
		// Convert back to new format
		newInfo := map[string]interface{}{
			"schemaVersion":       "1",
			"id":                  info["appid"],
			"arch":                info["arch"],
			"base":                info["base"],
			"channel":             info["channel"],
			"command":             info["command"],
			"description":         info["description"],
			"kind":                info["kind"],
			"name":                info["name"],
			"runtime":             info["runtime"],
			"size":                info["size"],
			"version":             info["version"],
			"packageInfoV2Module": info["module"],
		}

		infoJSON, err := json.MarshalIndent(newInfo, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal info: %w", err)
		}

		if err := os.WriteFile(filepath.Join(destDir, "info.json"), infoJSON, 0644); err != nil {
			return fmt.Errorf("failed to write info.json: %w", err)
		}
	}

	// Read remaining data as erofs
	erofsData, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("failed to read erofs data: %w", err)
	}

	// Create temporary erofs file
	tmpDir, err := os.MkdirTemp("", "linglong-unpack-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	erofsFile := filepath.Join(tmpDir, "layer.erofs")
	if err := os.WriteFile(erofsFile, erofsData, 0644); err != nil {
		return fmt.Errorf("failed to write erofs file: %w", err)
	}

	// Try to extract erofs
	if err := p.extractErofs(erofsFile, destDir); err != nil {
		// Fallback: try as tar
		if err := p.extractTar(erofsFile, destDir); err != nil {
			return fmt.Errorf("failed to extract layer: %w", err)
		}
	}

	return nil
}

// extractErofs extracts an erofs filesystem
func (p *Packager) extractErofs(erofsFile, destDir string) error {
	// Try fsck.erofs
	fsckErofs, err := exec.LookPath("fsck.erofs")
	if err != nil {
		return fmt.Errorf("fsck.erofs not found")
	}

	cmd := exec.Command(fsckErofs, "--extract="+destDir, erofsFile)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("fsck.erofs failed: %w", err)
	}

	return nil
}

// extractTar extracts a tar archive
func (p *Packager) extractTar(tarFile, destDir string) error {
	cmd := exec.Command("tar", "-xf", tarFile, "-C", destDir)
	return cmd.Run()
}

// unpackLegacy handles old format layers (just compressed tar)
func (p *Packager) unpackLegacy(file *os.File, destDir string) error {
	// Read first bytes to detect compression
	header := make([]byte, 16)
	n, _ := file.Read(header)
	file.Seek(0, 0)

	var reader io.Reader

	if n >= 4 {
		if header[0] == 0x04 && header[1] == 0x22 && header[2] == 0x4D && header[3] == 0x18 {
			reader = lz4.NewReader(file)
		} else if header[0] == 0x28 && header[1] == 0xB5 && header[2] == 0x2F && header[3] == 0xFD {
			decoder, err := zstd.NewReader(file)
			if err != nil {
				return fmt.Errorf("failed to create zstd reader: %w", err)
			}
			defer decoder.Close()
			reader = decoder
		} else {
			gzReader, err := pgzip.NewReader(file)
			if err != nil {
				return fmt.Errorf("failed to create gzip reader: %w", err)
			}
			defer gzReader.Close()
			reader = gzReader
		}
	}

	// Save to temp file and extract
	tmpFile, err := os.CreateTemp("", "linglong-*.tar")
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, reader); err != nil {
		return err
	}
	tmpFile.Close()

	cmd := exec.Command("tar", "-xf", tmpFile.Name(), "-C", destDir)
	return cmd.Run()
}

// Extract extracts a layer file to a directory (alias for Unpack)
func (p *Packager) Extract(layerFile, destDir string) error {
	return p.Unpack(layerFile, destDir)
}

// GetLayerFilename generates the layer filename
func GetLayerFilename(id, version, arch, module string) string {
	return fmt.Sprintf("%s_%s_%s_%s.layer", id, version, arch, module)
}

// copyDir copies a directory recursively
func copyDir(src, dst string) error {
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

		// Copy file with permissions
		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			return err
		}

		dstFile, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		defer dstFile.Close()

		_, err = io.Copy(dstFile, srcFile)
		return err
	})
}
