package acctest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v2"
)

// ResourceYAMLMetadata represents the structure of the metadata files
type ResourceYAMLMetadata struct {
	Resource       string `yaml:"resource"`
	ApiServiceName string `yaml:"api_service_name"`
	SourceFile     string `yaml:"source_file"`
}

// Cache structures to avoid repeated file system operations
var (
	// Cache for API service names (resourceName -> apiServiceName)
	apiServiceNameCache = make(map[string]string)
	// Cache for service packages (resourceType -> servicePackage)
	servicePackageCache = make(map[string]string)
	// Flag to track if cache has been populated
	cachePopulated = false
	// Mutex to protect cache access
	cacheMutex sync.RWMutex
)

// PopulateMetadataCache walks through all metadata files once and populates
// both the API service name and service package caches for improved performance
func PopulateMetadataCache() error {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()

	// If cache is already populated, we can skip
	if cachePopulated {
		return nil
	}

	baseDir, err := getServicesDir()
	if err != nil {
		return fmt.Errorf("failed to find services directory: %v", err)
	}

	// Count for statistics
	apiNameCount := 0
	servicePkgCount := 0

	// Walk through all service directories once
	err = filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files with errors but continue walking
		}

		// Look for metadata files
		if !info.IsDir() && strings.HasPrefix(info.Name(), "resource_") && strings.HasSuffix(info.Name(), "_meta.yaml") {
			// Read the file
			content, err := os.ReadFile(path)
			if err != nil {
				return nil // Continue to next file
			}

			// Parse YAML
			var metadata ResourceYAMLMetadata
			if err := yaml.Unmarshal(content, &metadata); err != nil {
				return nil // Continue to next file
			}

			// Skip if resource is empty
			if metadata.Resource == "" {
				return nil
			}

			// Store API service name in cache
			if metadata.ApiServiceName != "" {
				apiServiceNameCache[metadata.Resource] = metadata.ApiServiceName
				apiNameCount++
			}

			// Extract and store service package in cache
			pathParts := strings.Split(path, string(os.PathSeparator))
			servicesIndex := -1
			for i, part := range pathParts {
				if part == "services" {
					servicesIndex = i
					break
				}
			}

			if servicesIndex >= 0 && len(pathParts) > servicesIndex+1 {
				servicePackage := pathParts[servicesIndex+1] // The part after "services"
				servicePackageCache[metadata.Resource] = servicePackage
				servicePkgCount++
			}
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("error walking directory: %v", err)
	}

	// Mark cache as populated
	cachePopulated = true

	return nil
}

// GetAPIServiceNameForResource finds the api_service_name for a given resource name
// If projectRoot is empty, it will attempt to find the project root automatically
func GetAPIServiceNameForResource(resourceName string) string {
	// Make sure cache is populated
	if !cachePopulated {
		if err := PopulateMetadataCache(); err != nil {
			return "failed_to_populate_metadata_cache"
		}
	}

	// Check cache
	cacheMutex.RLock()
	apiServiceName, found := apiServiceNameCache[resourceName]
	cacheMutex.RUnlock()

	if !found {
		return "unknown"
	}

	return apiServiceName
}

// GetServicePackageForResourceType finds the service package for a given resource type
// If projectRoot is empty, it will attempt to find the project root automatically
func GetServicePackageForResourceType(resourceType string) string {
	// Make sure cache is populated
	if !cachePopulated {
		if err := PopulateMetadataCache(); err != nil {
			return "failed_to_populate_metadata_cache"
		}
	}

	// Check cache
	cacheMutex.RLock()
	servicePackage, found := servicePackageCache[resourceType]
	cacheMutex.RUnlock()

	if !found {
		return "unknown"
	}

	return servicePackage
}

// getServicesDir returns the path to the services directory
// It will attempt to find the project root relative to cwd
func getServicesDir() (string, error) {
	// Try to find project root
	root, err := findProjectRoot()
	if err == nil {
		servicesDir := filepath.Join(root, "google-beta", "services")
		if _, err := os.Stat(servicesDir); err == nil {
			return servicesDir, nil
		}
	}

	// Last resort: try relative to current directory
	currentDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to determine current directory: %v", err)
	}

	// Try a few common relative paths
	potentialPaths := []string{
		filepath.Join(currentDir, "google-beta", "services"),
		filepath.Join(currentDir, "..", "google-beta", "services"),
		filepath.Join(currentDir, "..", "..", "google-beta", "services"),
	}

	for _, path := range potentialPaths {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("unable to locate services directory, please provide explicit project root path")
}

// findProjectRoot walks up from the current directory to find the project root
// by looking for the go.mod file
func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		// Check if go.mod exists in the current directory
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		// Move up to the parent directory
		parentDir := filepath.Dir(dir)
		if parentDir == dir {
			// Reached the filesystem root without finding go.mod
			return "", fmt.Errorf("could not find go.mod file in any parent directory")
		}
		dir = parentDir
	}
}
