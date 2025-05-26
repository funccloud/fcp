package helm

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"k8s.io/cli-runtime/pkg/genericiooptions"
)

const (
	helmVersion = "v3.17.3" // Define a specific Helm version
	helmBaseURL = "https://get.helm.sh"
	goosWindows = "windows"
)

// checkHelmBinaryStatus checks if the Helm binary exists at the given path,
// and if it's executable on non-Windows systems.
// It returns true if the binary is ready to use, false otherwise, and an error if any occurred.
func checkHelmBinaryStatus(streams genericiooptions.IOStreams, helmPath string, fi os.FileInfo) (bool, error) {
	if runtime.GOOS == goosWindows {
		// On Windows, executability is typically determined by file extension (.exe)
		_, _ = fmt.Fprintf(streams.Out, "Helm binary already exists at %s\n", helmPath)
		return true, nil
	}

	// Non-Windows: Check if executable (any execute bit: owner, group, or other)
	if fi.Mode()&0111 != 0 {
		_, _ = fmt.Fprintf(streams.Out, "Helm binary already exists and is executable at %s\n", helmPath)
		return true, nil
	}

	// File exists but is not executable, make it executable
	_, _ = fmt.Fprintf(streams.Out, "Helm binary at %s exists but is not executable. Making it executable...\n", helmPath)
	if chmodErr := os.Chmod(helmPath, 0755); chmodErr != nil {
		return false, fmt.Errorf("failed to make helm binary executable: %w", chmodErr)
	}
	_, _ = fmt.Fprintf(streams.Out, "Helm binary at %s made executable.\n", helmPath)
	return true, nil
}

// EnsureHelmBinary checks if the Helm binary exists in the specified directory.
// If not, it downloads the appropriate version for the current OS/architecture
// and makes it executable.
// pluginDir is the directory where the helm binary should be placed.
func EnsureHelmBinary(streams genericiooptions.IOStreams, pluginDir string) error {
	helmBinaryName := "helm"
	if runtime.GOOS == goosWindows {
		helmBinaryName += ".exe"
	}
	helmPath := filepath.Join(pluginDir, helmBinaryName)

	// 1. Check if helm binary exists
	fi, err := os.Stat(helmPath)

	if err == nil { // File exists
		ready, checkErr := checkHelmBinaryStatus(streams, helmPath, fi)
		if checkErr != nil {
			return checkErr // Error from checkHelmBinaryStatus (e.g., chmod failed)
		}
		if ready {
			return nil // Binary is ready
		}
		// This state (not ready, no error from checkHelmBinaryStatus) should ideally not be reached
		// if checkHelmBinaryStatus is correctly implemented, as it returns (true, nil) on success
		// or (false, error) on chmod failure.
		return fmt.Errorf("helm binary at %s exists but was not made ready (unexpected state from check)", helmPath)
	}

	// If err is not nil, check if it's a "file not found" error.
	// Other errors (e.g. permission issues with os.Stat) are caught here.
	if !os.IsNotExist(err) {
		return fmt.Errorf("failed to check for helm binary at %s: %w", helmPath, err)
	}

	// At this point, os.IsNotExist(err) is true, so the binary does not exist.
	// 2. Helm binary does not exist, proceed to download
	_, _ = fmt.Fprintf(streams.Out, "Helm binary not found at %s. Downloading...\n", helmPath)

	// Ensure pluginDir exists, creating it if necessary
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		return fmt.Errorf("failed to create plugin directory %s: %w", pluginDir, err)
	}

	// Determine OS and architecture for the download URL
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	archiveName := fmt.Sprintf("helm-%s-%s-%s.tar.gz", helmVersion, goos, goarch)
	downloadURL := fmt.Sprintf("%s/%s", helmBaseURL, archiveName)

	_, _ = fmt.Fprintf(streams.Out, "Downloading Helm from %s\n", downloadURL)

	// 3. Download the tar.gz file
	resp, err := http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("failed to start download of helm from %s: %w", downloadURL, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			_, _ = fmt.Fprintf(streams.ErrOut, "Error closing response body: %s\n", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download helm from %s: received status code %d", downloadURL, resp.StatusCode)
	}

	// Create a temporary file to store the downloaded archive
	tmpFile, err := os.CreateTemp("", "helm-*.tar.gz")
	if err != nil {
		return fmt.Errorf("failed to create temporary file for helm download: %w", err)
	}
	defer func() {
		if err := os.Remove(tmpFile.Name()); err != nil {
			_, _ = fmt.Fprintf(streams.ErrOut, "Error removing temporary file %s: %s\n", tmpFile.Name(), err)
		}
	}() // Ensure temporary file is cleaned up

	_, err = io.Copy(tmpFile, resp.Body)
	if err != nil {
		if closeErr := tmpFile.Close(); closeErr != nil {
			_, _ = fmt.Fprintf(streams.ErrOut, "Error closing temporary file %s: %s\n", tmpFile.Name(), closeErr)
		}
		return fmt.Errorf("failed to save helm download to temporary file %s: %w", tmpFile.Name(), err)
	}
	if err := tmpFile.Close(); err != nil { // Close the file so it can be reopened for reading
		return fmt.Errorf("failed to close temporary file %s: %w", tmpFile.Name(), err)
	}

	// 4. Extract the helm binary from the tar.gz archive
	fileReader, err := os.Open(tmpFile.Name())
	if err != nil {
		return fmt.Errorf("failed to open downloaded helm archive %s for reading: %w", tmpFile.Name(), err)
	}
	defer func() {
		if err := fileReader.Close(); err != nil {
			_, _ = fmt.Fprintf(streams.ErrOut, "Error closing file reader for %s: %s\n", tmpFile.Name(), err)
		}
	}()

	gzipReader, err := gzip.NewReader(fileReader)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader for helm archive %s: %w", tmpFile.Name(), err)
	}
	defer func() {
		if err := gzipReader.Close(); err != nil {
			_, _ = fmt.Fprintf(streams.ErrOut, "Error closing gzip reader for %s: %s\n", tmpFile.Name(), err)
		}
	}()

	tarReader := tar.NewReader(gzipReader)

	// The Helm binary is typically located at "<os>-<arch>/helm" within the archive
	expectedTarPath := fmt.Sprintf("%s-%s/%s", goos, goarch, helmBinaryName)
	foundHelmBinary := false

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			return fmt.Errorf("failed to read from helm tar archive %s: %w", tmpFile.Name(), err)
		}

		if header.Name == expectedTarPath {
			// Found the helm binary, create the output file
			outFile, err := os.OpenFile(helmPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("failed to create helm binary file at %s: %w", helmPath, err)
			}

			if _, err := io.Copy(outFile, tarReader); err != nil {
				if closeErr := outFile.Close(); closeErr != nil { // Close before returning error
					_, _ = fmt.Fprintf(streams.ErrOut, "Error closing output file %s: %s\n", helmPath, closeErr)
				}
				return fmt.Errorf("failed to extract helm binary from archive to %s: %w", helmPath, err)
			}
			if err := outFile.Close(); err != nil { // Successfully wrote the file
				return fmt.Errorf("failed to close output file %s: %w", helmPath, err)
			}
			foundHelmBinary = true
			_, _ = fmt.Fprintf(streams.Out, "Helm binary extracted from %s in archive to %s\n", header.Name, helmPath)
			break // Binary found and extracted
		}
	}

	if !foundHelmBinary {
		return fmt.Errorf("helm binary not found within downloaded archive %s (expected at path %s)",
			archiveName, expectedTarPath)
	}

	// 5. Make the binary executable (on non-Windows systems)
	if runtime.GOOS != goosWindows {
		if err := os.Chmod(helmPath, 0755); err != nil {
			return fmt.Errorf("failed to make helm binary %s executable: %w", helmPath, err)
		}
		_, _ = fmt.Fprintf(streams.Out, "Helm binary at %s made executable.\n", helmPath)
	}

	_, _ = fmt.Fprintf(streams.Out, "Helm binary setup complete at %s.\n", helmPath)
	return nil
}
