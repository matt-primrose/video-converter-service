package storage

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/matt-primrose/video-converter-service/internal/config"
)

// AzureStorage implements the Storage interface for Azure Blob Storage
type AzureStorage struct {
	config         StorageConfig
	account        string
	container      string
	accountKey     string
	endpointSuffix string
	client         *azblob.Client
}

// NewAzureStorage creates a new Azure Blob Storage instance
func NewAzureStorage(azureConfig config.AzureBlobStorage, storageConfig StorageConfig) (*AzureStorage, error) {
	// Set default endpoint suffix if not provided
	endpointSuffix := azureConfig.EndpointSuffix
	if endpointSuffix == "" {
		endpointSuffix = "core.windows.net"
	}

	storage := &AzureStorage{
		config:         storageConfig,
		account:        azureConfig.Account,
		container:      azureConfig.Container,
		accountKey:     azureConfig.AccountKey,
		endpointSuffix: endpointSuffix,
	}

	// Initialize Azure client if we have credentials
	if azureConfig.AccountKey != "" {
		if err := storage.initializeClient(); err != nil {
			return nil, fmt.Errorf("failed to initialize Azure client: %w", err)
		}
	}

	return storage, nil
}

// initializeClient creates the Azure Blob client with authentication
func (as *AzureStorage) initializeClient() error {
	// Build connection string
	connectionString := fmt.Sprintf(
		"DefaultEndpointsProtocol=https;AccountName=%s;AccountKey=%s;EndpointSuffix=%s",
		as.account,
		as.accountKey,
		as.endpointSuffix,
	)

	client, err := azblob.NewClientFromConnectionString(connectionString, nil)
	if err != nil {
		return fmt.Errorf("failed to create Azure client: %w", err)
	}

	as.client = client
	return nil
}

// DownloadFile downloads a file from Azure Blob Storage
func (as *AzureStorage) DownloadFile(ctx context.Context, sourceURI string, jobID string) (string, error) {
	// Parse Azure Blob URL
	storageAccount, containerName, blobName, err := as.parseAzureBlobURL(sourceURI)
	if err != nil {
		return "", fmt.Errorf("invalid Azure blob URI: %w", err)
	}

	// Create temp directory for this job
	tempDir := filepath.Join(as.config.TempDir, jobID)
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Create temp file path
	ext := filepath.Ext(blobName)
	tempFilePath := filepath.Join(tempDir, "source"+ext)

	slog.Info("Azure Blob download details",
		"jobId", jobID,
		"storageAccount", storageAccount,
		"container", containerName,
		"blobName", blobName,
	)

	// Try authenticated download first if we have credentials
	if as.client != nil {
		if err := as.downloadAuthenticatedBlob(ctx, containerName, blobName, tempFilePath); err != nil {
			slog.Warn("Authenticated download failed, trying public access", "error", err)
		} else {
			return tempFilePath, nil
		}
	}

	// Fallback to public blob access via HTTP
	if err := as.downloadPublicBlob(ctx, sourceURI, tempFilePath); err != nil {
		return "", fmt.Errorf("failed to download Azure blob: %w", err)
	}

	slog.Info("Successfully downloaded Azure blob",
		"jobId", jobID,
		"blobName", blobName,
		"tempPath", tempFilePath,
	)

	return tempFilePath, nil
}

// UploadFile uploads a file to Azure Blob Storage
func (as *AzureStorage) UploadFile(ctx context.Context, sourcePath string, destinationPath string) error {
	if as.client == nil {
		return fmt.Errorf("azure client not initialized - missing credentials")
	}

	// Open source file
	file, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer file.Close()

	// Upload to Azure Blob
	_, err = as.client.UploadStream(ctx, as.container, destinationPath, file, nil)
	if err != nil {
		return fmt.Errorf("failed to upload to Azure Blob: %w", err)
	}

	slog.Info("Successfully uploaded file to Azure Blob Storage",
		"sourcePath", sourcePath,
		"container", as.container,
		"blobName", destinationPath,
	)

	return nil
}

// UploadFiles uploads multiple files to Azure Blob Storage
func (as *AzureStorage) UploadFiles(ctx context.Context, fileMap map[string]string) error {
	for sourcePath, destinationPath := range fileMap {
		if err := as.UploadFile(ctx, sourcePath, destinationPath); err != nil {
			return fmt.Errorf("failed to upload file %s: %w", sourcePath, err)
		}
	}
	return nil
}

// GetFileURL returns a public URL for the Azure blob
func (as *AzureStorage) GetFileURL(destinationPath string) (string, error) {
	return fmt.Sprintf("https://%s.blob.%s/%s/%s",
		as.account,
		as.endpointSuffix,
		as.container,
		destinationPath,
	), nil
}

// DeleteFile deletes a file from Azure Blob Storage
func (as *AzureStorage) DeleteFile(ctx context.Context, destinationPath string) error {
	if as.client == nil {
		return fmt.Errorf("azure client not initialized - missing credentials")
	}

	_, err := as.client.DeleteBlob(ctx, as.container, destinationPath, nil)
	if err != nil {
		return fmt.Errorf("failed to delete Azure blob: %w", err)
	}

	slog.Debug("Deleted file from Azure Blob Storage",
		"container", as.container,
		"blobName", destinationPath,
	)

	return nil
}

// ListFiles lists files in Azure Blob Storage with a prefix
func (as *AzureStorage) ListFiles(ctx context.Context, prefix string) ([]string, error) {
	if as.client == nil {
		return nil, fmt.Errorf("azure client not initialized - missing credentials")
	}

	var files []string
	pager := as.client.NewListBlobsFlatPager(as.container, &azblob.ListBlobsFlatOptions{
		Prefix: &prefix,
	})

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list Azure blobs: %w", err)
		}

		for _, blob := range page.Segment.BlobItems {
			if blob.Name != nil {
				files = append(files, *blob.Name)
			}
		}
	}

	return files, nil
}

// GetType returns the storage type
func (as *AzureStorage) GetType() string {
	return "azure-blob"
}

// parseAzureBlobURL parses an Azure Blob Storage URL and extracts components
func (as *AzureStorage) parseAzureBlobURL(blobURI string) (storageAccount, containerName, blobName string, err error) {
	parsedURL, err := url.Parse(blobURI)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to parse URL: %w", err)
	}

	// Extract storage account from hostname (e.g., "mystorageaccount.blob.core.windows.net")
	hostParts := strings.Split(parsedURL.Host, ".")
	if len(hostParts) < 2 {
		return "", "", "", fmt.Errorf("invalid Azure blob hostname format")
	}
	storageAccount = hostParts[0]

	// Extract container and blob name from path (e.g., "/container/path/to/blob.mp4")
	pathParts := strings.SplitN(strings.Trim(parsedURL.Path, "/"), "/", 2)
	if len(pathParts) < 2 {
		return "", "", "", fmt.Errorf("invalid Azure blob path format")
	}
	containerName = pathParts[0]
	blobName = pathParts[1]

	return storageAccount, containerName, blobName, nil
}

// downloadAuthenticatedBlob downloads a blob using Azure SDK with authentication
func (as *AzureStorage) downloadAuthenticatedBlob(ctx context.Context, containerName, blobName, tempFilePath string) error {
	// Download the blob
	response, err := as.client.DownloadStream(ctx, containerName, blobName, nil)
	if err != nil {
		return fmt.Errorf("failed to download blob via Azure SDK: %w", err)
	}
	defer response.Body.Close()

	// Create output file
	outFile, err := os.Create(tempFilePath)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer outFile.Close()

	// Copy data
	if _, err := io.Copy(outFile, response.Body); err != nil {
		return fmt.Errorf("failed to write blob data: %w", err)
	}

	return nil
}

// downloadPublicBlob downloads a blob that has public read access via HTTP
func (as *AzureStorage) downloadPublicBlob(ctx context.Context, blobURI, tempFilePath string) error {
	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", blobURI, nil)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Make request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download blob: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("blob download failed with status: %d", resp.StatusCode)
	}

	// Create output file
	outFile, err := os.Create(tempFilePath)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer outFile.Close()

	// Copy data
	if _, err := io.Copy(outFile, resp.Body); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}
