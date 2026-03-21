package source

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/OpenAtom-Linyaps/linyaps/golang/ll-builder/internal/types"
)

// Fetcher handles source code fetching
type Fetcher struct {
	cacheDir string
}

// NewFetcher creates a new source fetcher
func NewFetcher(cacheDir string) *Fetcher {
	return &Fetcher{cacheDir: cacheDir}
}

// Fetch fetches a source to the destination directory
func (f *Fetcher) Fetch(source types.BuilderProjectSource, destination string) error {
	if err := os.MkdirAll(destination, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	if source.URL == nil {
		return fmt.Errorf("URL is missing")
	}

	sourceName := f.getSourceName(source)
	destPath := filepath.Join(destination, sourceName)

	switch source.Kind {
	case "archive":
		return f.fetchArchive(source, destPath)
	case "git":
		return f.fetchGit(source, destPath)
	case "file":
		return f.fetchFile(source, destPath)
	case "dsc":
		return f.fetchDsc(source, destPath)
	default:
		return fmt.Errorf("unknown source kind: %s", source.Kind)
	}
}

// FetchAll fetches all sources to the destination directory
func (f *Fetcher) FetchAll(sources []types.BuilderProjectSource, destination string) error {
	if err := os.MkdirAll(destination, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	for i, src := range sources {
		fmt.Printf("Source %-15s %-15s %-75s downloading ...\n",
			fmt.Sprintf("%d", i), src.Kind, truncateURL(*src.URL, 75))

		if err := f.Fetch(src, destination); err != nil {
			return fmt.Errorf("failed to fetch source %d: %w", i, err)
		}

		fmt.Printf("Source %-15s %-15s %-75s complete\n",
			fmt.Sprintf("%d", i), src.Kind, truncateURL(*src.URL, 75))
	}

	return nil
}

func (f *Fetcher) getSourceName(source types.BuilderProjectSource) string {
	if source.Name != nil {
		return *source.Name
	}
	if source.URL != nil {
		parts := strings.Split(*source.URL, "/")
		return parts[len(parts)-1]
	}
	return "unknown"
}

func (f *Fetcher) fetchArchive(source types.BuilderProjectSource, destPath string) error {
	if source.Digest == nil {
		return fmt.Errorf("digest is required for archive source")
	}

	// Check cache first
	cachePath := filepath.Join(f.cacheDir, filepath.Base(destPath))
	if f.checkCache(cachePath, *source.Digest) {
		fmt.Printf("Using cached file: %s\n", cachePath)
		return f.extractArchive(cachePath, filepath.Dir(destPath))
	}

	// Download file
	if err := f.downloadFile(*source.URL, cachePath); err != nil {
		return err
	}

	// Verify digest
	if err := f.verifyDigest(cachePath, *source.Digest); err != nil {
		os.Remove(cachePath)
		return err
	}

	// Extract archive
	return f.extractArchive(cachePath, filepath.Dir(destPath))
}

func (f *Fetcher) fetchGit(source types.BuilderProjectSource, destPath string) error {
	if source.Commit == nil {
		return fmt.Errorf("commit is required for git source")
	}

	// Check if already cloned
	if _, err := os.Stat(filepath.Join(destPath, ".git")); err == nil {
		// Already exists, checkout specific commit
		cmd := exec.Command("git", "checkout", *source.Commit)
		cmd.Dir = destPath
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to checkout commit: %w", err)
		}
		return nil
	}

	// Clone repository
	fmt.Printf("Cloning %s...\n", *source.URL)
	cmd := exec.Command("git", "clone", *source.URL, destPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}

	// Checkout specific commit
	cmd = exec.Command("git", "checkout", *source.Commit)
	cmd.Dir = destPath
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to checkout commit: %w", err)
	}

	// Handle submodules if enabled
	if source.Submodules == nil || *source.Submodules {
		cmd = exec.Command("git", "submodule", "update", "--init", "--recursive")
		cmd.Dir = destPath
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Printf("Warning: failed to update submodules: %v\n", err)
		}
	}

	return nil
}

func (f *Fetcher) fetchFile(source types.BuilderProjectSource, destPath string) error {
	if source.Digest == nil {
		return fmt.Errorf("digest is required for file source")
	}

	// Check cache first
	cachePath := filepath.Join(f.cacheDir, filepath.Base(destPath))
	if f.checkCache(cachePath, *source.Digest) {
		fmt.Printf("Using cached file: %s\n", cachePath)
		return f.copyFile(cachePath, destPath)
	}

	// Download file
	if err := f.downloadFile(*source.URL, cachePath); err != nil {
		return err
	}

	// Verify digest
	if err := f.verifyDigest(cachePath, *source.Digest); err != nil {
		os.Remove(cachePath)
		return err
	}

	// Copy to destination
	return f.copyFile(cachePath, destPath)
}

func (f *Fetcher) fetchDsc(source types.BuilderProjectSource, destPath string) error {
	if source.Digest == nil {
		return fmt.Errorf("digest is required for dsc source")
	}

	// DSC files are Debian source packages
	// Download the .dsc file first
	dscPath := filepath.Join(f.cacheDir, filepath.Base(*source.URL))
	if !f.checkCache(dscPath, *source.Digest) {
		if err := f.downloadFile(*source.URL, dscPath); err != nil {
			return err
		}
		if err := f.verifyDigest(dscPath, *source.Digest); err != nil {
			os.Remove(dscPath)
			return err
		}
	}

	// Use dpkg-source to extract
	cmd := exec.Command("dpkg-source", "-x", dscPath, destPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to extract dsc: %w", err)
	}

	return nil
}

func (f *Fetcher) downloadFile(url, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download file: status %d", resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

func (f *Fetcher) checkCache(path, digest string) bool {
	if _, err := os.Stat(path); err != nil {
		return false
	}
	return f.verifyDigest(path, digest) == nil
}

func (f *Fetcher) verifyDigest(path, digest string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("failed to calculate hash: %w", err)
	}

	calculated := hex.EncodeToString(hash.Sum(nil))
	expected := strings.TrimPrefix(digest, "sha256:")

	if calculated != expected {
		return fmt.Errorf("digest mismatch: expected %s, got %s", expected, calculated)
	}

	return nil
}

func (f *Fetcher) extractArchive(archivePath, destDir string) error {
	// Create destination directory
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination: %w", err)
	}

	// Detect archive type and extract
	var cmd *exec.Cmd
	switch {
	case strings.HasSuffix(archivePath, ".tar.gz") || strings.HasSuffix(archivePath, ".tgz"):
		cmd = exec.Command("tar", "-xzf", archivePath, "-C", destDir)
	case strings.HasSuffix(archivePath, ".tar.bz2") || strings.HasSuffix(archivePath, ".tbz2"):
		cmd = exec.Command("tar", "-xjf", archivePath, "-C", destDir)
	case strings.HasSuffix(archivePath, ".tar.xz") || strings.HasSuffix(archivePath, ".txz"):
		cmd = exec.Command("tar", "-xJf", archivePath, "-C", destDir)
	case strings.HasSuffix(archivePath, ".zip"):
		cmd = exec.Command("unzip", "-q", archivePath, "-d", destDir)
	default:
		cmd = exec.Command("tar", "-xf", archivePath, "-C", destDir)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to extract archive: %w", err)
	}

	return nil
}

func (f *Fetcher) copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}

func truncateURL(url string, maxLen int) string {
	if len(url) <= maxLen {
		return url
	}
	return "..." + url[len(url)-maxLen+3:]
}
