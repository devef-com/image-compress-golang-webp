package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

func main() {
	router := gin.Default()

	//TODO remove after adding domain
	// CORS middleware
	router.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	})

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

	log.Printf("Starting server on port %s", port)
	router.Run(":" + port)
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

	// Get quality parameter (default: 80)
	quality := c.DefaultQuery("quality", "80")

	// Convert to WebP using cwebp (from apt package)
	cmd := exec.Command("cwebp", "-q", quality, inputPath, "-o", outputPath)
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
