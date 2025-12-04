package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

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

	router.POST("/convert", convertToWebP)

	router.Run(":8080")
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

	// Get libwebp path from environment
	libwebpPath := os.Getenv("LIBWEBP_PATH")
	if libwebpPath == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "LIBWEBP_PATH environment variable not set"})
		return
	}

	// Get the working directory to construct absolute path
	workDir, err := os.Getwd()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get working directory"})
		return
	}

	// Construct the cwebp binary path
	cwebpPath := filepath.Join(workDir, libwebpPath, "bin", "cwebp")

	// Convert to WebP using cwebp
	cmd := exec.Command(cwebpPath, "-q", "80", inputPath, "-o", outputPath)
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
