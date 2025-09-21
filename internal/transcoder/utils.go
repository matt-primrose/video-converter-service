package transcoder

import (
	"crypto/md5"
	"fmt"
	"io"
	"os"

	"github.com/matt-primrose/video-converter-service/pkg/models"
)

// createOutputFile creates an OutputFile struct with file information
func (t *Transcoder) createOutputFile(filePath, mimeType string) (*models.OutputFile, error) {
	// Get file info
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	// Calculate file checksum
	checksum, err := t.calculateChecksum(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate checksum: %w", err)
	}

	return &models.OutputFile{
		Path:     filePath,
		Size:     fileInfo.Size(),
		Checksum: checksum,
		MimeType: mimeType,
	}, nil
}

// calculateChecksum calculates MD5 checksum of a file
func (t *Transcoder) calculateChecksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}
