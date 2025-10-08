package storage

import (
	"fmt"

	"github.com/matt-primrose/video-converter-service/internal/config"
)

// Factory creates storage instances based on configuration
func NewStorage(cfg *config.Config) (Storage, error) {
	storageConfig := StorageConfig{
		TempDir:    cfg.Processing.TempDir,
		OutputsDir: cfg.Processing.OutputsDir,
	}

	switch cfg.Storage.Type {
	case "local":
		return NewLocalStorage(cfg.Storage.Local.Path, storageConfig), nil

	case "azure-blob":
		return NewAzureStorage(cfg.Storage.AzureBlob, storageConfig)

	case "s3":
		s3Config := S3Config{
			Bucket: cfg.Storage.S3.Bucket,
			Region: cfg.Storage.S3.Region,
		}
		return NewS3Storage(s3Config, storageConfig)

	default:
		return nil, fmt.Errorf("unsupported storage type: %s", cfg.Storage.Type)
	}
}

// NewDownloadOnlyStorage creates storage instances specifically for downloading from different sources
// This is useful for the worker when it needs to download from various sources regardless of output storage type
func NewDownloadOnlyStorage(sourceType string, cfg *config.Config) (Storage, error) {
	storageConfig := StorageConfig{
		TempDir:    cfg.Processing.TempDir,
		OutputsDir: cfg.Processing.OutputsDir,
	}

	switch sourceType {
	case "local":
		// For local downloads, use a temporary local storage instance
		return NewLocalStorage("", storageConfig), nil

	case "azure-blob":
		// Create Azure storage for downloading - use the same config as output storage
		// In production, you might want separate download credentials
		azureConfig := config.AzureBlobStorage{
			Account:        cfg.Storage.AzureBlob.Account,
			Container:      "", // Container will be parsed from URL
			AccountKey:     cfg.Storage.AzureBlob.AccountKey,
			EndpointSuffix: cfg.Storage.AzureBlob.EndpointSuffix,
		}
		return NewAzureStorage(azureConfig, storageConfig)

	case "s3":
		s3Config := S3Config{
			Bucket: "", // Bucket will be parsed from URL
			Region: cfg.Storage.S3.Region,
		}
		return NewS3Storage(s3Config, storageConfig)

	case "http", "https":
		// For HTTP downloads, use HTTP storage implementation
		return NewHTTPStorage(storageConfig), nil

	default:
		return nil, fmt.Errorf("unsupported source type for download: %s", sourceType)
	}
}
