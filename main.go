package main

import (
	"fmt"
	"io"
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
		fmt.Println("Warning: .env file not found")
	}

	router := gin.Default()

	// Set max upload size (10 MB)
	router.MaxMultipartMemory = 10 << 20

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Convert and return WebP directly
	router.POST("/convert", convertToWebP)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

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

	// Default path for Railway deployment
	return "/app/libwebp/bin/cwebp", nil
}

// convertToWebP handles image upload and converts it to WebP format
func convertToWebP(c *gin.Context) {
	// Get the uploaded file
	file, header, err := c.Request.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No image file provided"})
		return
	}
	defer file.Close()

	// Create a temporary directory for processing
	tempDir, err := os.MkdirTemp("", "webp-convert-*")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create temp directory"})
		return
	}
	defer os.RemoveAll(tempDir)

	// Save the uploaded file temporarily
	inputPath := filepath.Join(tempDir, header.Filename)
	inputFile, err := os.Create(inputPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save uploaded file"})
		return
	}

	_, err = io.Copy(inputFile, file)
	inputFile.Close()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to copy uploaded file"})
		return
	}

	// Get the output filename (same name but with .webp extension)
	outputFilename := filenameWithoutExt(header.Filename) + ".webp"
	outputPath := filepath.Join(tempDir, outputFilename)

	// Get cwebp binary path
	cwebpPath, err := getCwebpPath()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Get quality parameter (default: 80)
	quality := c.DefaultQuery("quality", "80")

	// Convert to WebP using cwebp
	cmd := exec.Command(cwebpPath, "-q", quality, inputPath, "-o", outputPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to convert image",
			"details": string(output),
		})
		return
	}

	// Read the converted WebP file
	webpData, err := os.ReadFile(outputPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read converted file"})
		return
	}

	// Set response headers and send the WebP file
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", outputFilename))
	c.Data(http.StatusOK, "image/webp", webpData)
}

// filenameWithoutExt returns the filename without its extension
func filenameWithoutExt(filename string) string {
	ext := filepath.Ext(filename)
	return filename[:len(filename)-len(ext)]
}
