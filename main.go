package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found")
	}

	// Log the cwebp path on startup
	cwebpPath, err := getCwebpPath()
	if err != nil {
		log.Printf("Warning: Failed to get cwebp path: %v", err)
	} else {
		log.Printf("Using cwebp at: %s", cwebpPath)
		// Check if cwebp exists
		if _, err := os.Stat(cwebpPath); os.IsNotExist(err) {
			log.Printf("ERROR: cwebp binary not found at %s", cwebpPath)
		} else {
			log.Printf("cwebp binary found at %s", cwebpPath)
		}
	}

	router := gin.Default()

	// Set max upload size (10 MB)
	router.MaxMultipartMemory = 10 << 20

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Debug endpoint to check cwebp
	router.GET("/debug", func(c *gin.Context) {
		cwebpPath, err := getCwebpPath()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// Check if file exists
		info, statErr := os.Stat(cwebpPath)
		exists := statErr == nil

		// Try to get version
		var version string
		if exists {
			cmd := exec.Command(cwebpPath, "-version")
			output, err := cmd.CombinedOutput()
			if err != nil {
				version = fmt.Sprintf("error: %v - %s", err, string(output))
			} else {
				version = strings.TrimSpace(string(output))
			}
		}

		// List /app/libwebp directory
		var libwebpContents []string
		libwebpDir := "/app/libwebp"
		if entries, err := os.ReadDir(libwebpDir); err == nil {
			for _, entry := range entries {
				libwebpContents = append(libwebpContents, entry.Name())
			}
		} else {
			libwebpContents = []string{fmt.Sprintf("error reading dir: %v", err)}
		}

		// List /app/libwebp/bin directory
		var binContents []string
		binDir := "/app/libwebp/bin"
		if entries, err := os.ReadDir(binDir); err == nil {
			for _, entry := range entries {
				binContents = append(binContents, entry.Name())
			}
		} else {
			binContents = []string{fmt.Sprintf("error reading dir: %v", err)}
		}

		c.JSON(http.StatusOK, gin.H{
			"cwebpPath":       cwebpPath,
			"exists":          exists,
			"fileInfo":        fmt.Sprintf("%+v", info),
			"version":         version,
			"libwebpContents": libwebpContents,
			"binContents":     binContents,
		})
	})

	// Convert and return WebP directly
	router.POST("/convert", convertToWebP)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting server on port %s", port)
	router.Run(":" + port)
}

// getCwebpPath returns the path to cwebp binary based on environment
func getCwebpPath() (string, error) {
	// Check LIBWEBP_PATH environment variable first
	libwebpPath := os.Getenv("LIBWEBP_PATH")

	if libwebpPath != "" {
		// If it's an absolute path, use it directly
		if strings.HasPrefix(libwebpPath, "/") {
			return filepath.Join(libwebpPath, "bin", "cwebp"), nil
		}

		// Otherwise, construct relative to working directory
		workDir, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to get working directory: %w", err)
		}
		return filepath.Join(workDir, libwebpPath, "bin", "cwebp"), nil
	}

	// Default: use system cwebp (from apt package)
	return "cwebp", nil
}

// convertToWebP handles image upload and converts it to WebP format
func convertToWebP(c *gin.Context) {
	// Get the uploaded file
	file, header, err := c.Request.FormFile("image")
	if err != nil {
		log.Printf("Error getting form file: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "No image file provided"})
		return
	}
	defer file.Close()

	log.Printf("Received file: %s, size: %d", header.Filename, header.Size)

	// Create a temporary directory for processing
	tempDir, err := os.MkdirTemp("", "webp-convert-*")
	if err != nil {
		log.Printf("Error creating temp directory: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create temp directory"})
		return
	}
	defer os.RemoveAll(tempDir)

	// Save the uploaded file temporarily
	inputPath := filepath.Join(tempDir, header.Filename)
	inputFile, err := os.Create(inputPath)
	if err != nil {
		log.Printf("Error creating input file: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save uploaded file"})
		return
	}

	_, err = io.Copy(inputFile, file)
	inputFile.Close()
	if err != nil {
		log.Printf("Error copying file: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to copy uploaded file"})
		return
	}

	// Get the output filename (same name but with .webp extension)
	outputFilename := filenameWithoutExt(header.Filename) + ".webp"
	outputPath := filepath.Join(tempDir, outputFilename)

	// Get cwebp binary path
	cwebpPath, err := getCwebpPath()
	if err != nil {
		log.Printf("Error getting cwebp path: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Get quality parameter (default: 80)
	quality := c.DefaultQuery("quality", "80")

	log.Printf("Converting %s to WebP with quality %s using %s", inputPath, quality, cwebpPath)

	// Convert to WebP using cwebp
	cmd := exec.Command(cwebpPath, "-q", quality, inputPath, "-o", outputPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Error converting image: %v, output: %s", err, string(output))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to convert image",
			"details": string(output),
		})
		return
	}

	log.Printf("Conversion successful, reading output file")

	// Read the converted WebP file
	webpData, err := os.ReadFile(outputPath)
	if err != nil {
		log.Printf("Error reading converted file: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read converted file"})
		return
	}

	log.Printf("Sending WebP file: %s, size: %d bytes", outputFilename, len(webpData))

	// Set response headers and send the WebP file
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", outputFilename))
	c.Data(http.StatusOK, "image/webp", webpData)
}

// filenameWithoutExt returns the filename without its extension
func filenameWithoutExt(filename string) string {
	ext := filepath.Ext(filename)
	return filename[:len(filename)-len(ext)]
}
