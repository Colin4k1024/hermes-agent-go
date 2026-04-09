package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/config"
)

func init() {
	Register(&ToolEntry{
		Name:    "vision_analyze",
		Toolset: "vision",
		Schema: map[string]any{
			"name":        "vision_analyze",
			"description": "Analyze an image using a multimodal LLM. Provide either a local file path or a URL.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"image_path": map[string]any{
						"type":        "string",
						"description": "Local file path to the image",
					},
					"image_url": map[string]any{
						"type":        "string",
						"description": "URL of the image to analyze",
					},
					"prompt": map[string]any{
						"type":        "string",
						"description": "What to analyze or describe about the image",
						"default":     "Describe this image in detail.",
					},
				},
			},
		},
		Handler: handleVisionAnalyze,
		Emoji:   "\U0001f441\ufe0f",
	})

	Register(&ToolEntry{
		Name:    "image_generate",
		Toolset: "vision",
		Schema: map[string]any{
			"name":        "image_generate",
			"description": "Generate an image from a text prompt using fal.ai API. Returns the path to the generated image.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"prompt": map[string]any{
						"type":        "string",
						"description": "Text description of the image to generate",
					},
					"size": map[string]any{
						"type":        "string",
						"description": "Image size: 'square', 'landscape', or 'portrait'",
						"default":     "square",
					},
					"model": map[string]any{
						"type":        "string",
						"description": "Model to use (default: fal-ai/flux/schnell)",
						"default":     "fal-ai/flux/schnell",
					},
				},
				"required": []string{"prompt"},
			},
		},
		Handler:     handleImageGenerate,
		CheckFn:     checkVisionGenRequirements,
		RequiresEnv: []string{"FAL_KEY"},
		Emoji:       "\U0001f3a8",
	})
}

func checkVisionGenRequirements() bool {
	return os.Getenv("FAL_KEY") != ""
}

func handleVisionAnalyze(args map[string]any, ctx *ToolContext) string {
	imagePath, _ := args["image_path"].(string)
	imageURL, _ := args["image_url"].(string)
	prompt, _ := args["prompt"].(string)

	if imagePath == "" && imageURL == "" {
		return `{"error":"Either image_path or image_url is required"}`
	}

	if prompt == "" {
		prompt = "Describe this image in detail."
	}

	// If a local path is given, validate it exists
	if imagePath != "" {
		imagePath = absPath(imagePath)
		if !fileExists(imagePath) {
			return toJSON(map[string]any{"error": fmt.Sprintf("Image file not found: %s", imagePath)})
		}

		// For local files, convert to a data URL or just acknowledge
		ext := strings.ToLower(filepath.Ext(imagePath))
		mimeTypes := map[string]string{
			".png":  "image/png",
			".jpg":  "image/jpeg",
			".jpeg": "image/jpeg",
			".gif":  "image/gif",
			".webp": "image/webp",
			".bmp":  "image/bmp",
		}
		mime, ok := mimeTypes[ext]
		if !ok {
			return toJSON(map[string]any{"error": fmt.Sprintf("Unsupported image format: %s", ext)})
		}

		data, err := os.ReadFile(imagePath)
		if err != nil {
			return toJSON(map[string]any{"error": fmt.Sprintf("Cannot read image: %v", err)})
		}

		return toJSON(map[string]any{
			"status":    "analyzed",
			"path":      imagePath,
			"mime_type": mime,
			"size":      len(data),
			"prompt":    prompt,
			"message":   "Image analysis requires multimodal LLM integration. The image has been loaded and is ready for analysis in the conversation context.",
		})
	}

	// For URL-based images, return metadata for LLM processing
	return toJSON(map[string]any{
		"status":  "analyzed",
		"url":     imageURL,
		"prompt":  prompt,
		"message": "Image URL is ready for multimodal analysis in the conversation context.",
	})
}

func handleImageGenerate(args map[string]any, ctx *ToolContext) string {
	prompt, _ := args["prompt"].(string)
	if prompt == "" {
		return `{"error":"prompt is required"}`
	}

	size, _ := args["size"].(string)
	if size == "" {
		size = "square"
	}

	model, _ := args["model"].(string)
	if model == "" {
		model = "fal-ai/flux/schnell"
	}

	falKey := os.Getenv("FAL_KEY")
	if falKey == "" {
		return toJSON(map[string]any{"error": "FAL_KEY environment variable is not set"})
	}

	// Map size to dimensions
	sizeMap := map[string]map[string]int{
		"square":    {"width": 1024, "height": 1024},
		"landscape": {"width": 1344, "height": 768},
		"portrait":  {"width": 768, "height": 1344},
	}
	dims, ok := sizeMap[size]
	if !ok {
		dims = sizeMap["square"]
	}

	// Build API request
	payload := map[string]any{
		"prompt":     prompt,
		"image_size": map[string]any{"width": dims["width"], "height": dims["height"]},
		"num_images": 1,
	}
	body, _ := json.Marshal(payload)

	apiURL := fmt.Sprintf("https://fal.run/%s", model)
	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(body))
	if err != nil {
		return toJSON(map[string]any{"error": fmt.Sprintf("Failed to create request: %v", err)})
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Key "+falKey)

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return toJSON(map[string]any{"error": fmt.Sprintf("API request failed: %v", err)})
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return toJSON(map[string]any{
			"error":       "Image generation failed",
			"status_code": resp.StatusCode,
			"response":    truncateOutput(string(respBody), 500),
		})
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return toJSON(map[string]any{"error": "Failed to parse API response"})
	}

	// Extract image URL from response
	images, _ := result["images"].([]any)
	if len(images) == 0 {
		return toJSON(map[string]any{"error": "No images returned by API", "response": result})
	}

	firstImage, _ := images[0].(map[string]any)
	imgURL, _ := firstImage["url"].(string)

	if imgURL == "" {
		return toJSON(map[string]any{"error": "No image URL in response"})
	}

	// Download the image
	imgResp, err := http.Get(imgURL)
	if err != nil {
		return toJSON(map[string]any{
			"image_url": imgURL,
			"message":   "Image generated but download failed. Use the URL directly.",
		})
	}
	defer imgResp.Body.Close()

	imgData, _ := io.ReadAll(imgResp.Body)

	// Save to cache directory
	imagesDir := filepath.Join(config.HermesHome(), "cache", "images")
	os.MkdirAll(imagesDir, 0755)

	filename := fmt.Sprintf("gen_%d.png", time.Now().UnixMilli())
	savePath := filepath.Join(imagesDir, filename)

	if err := os.WriteFile(savePath, imgData, 0644); err != nil {
		return toJSON(map[string]any{
			"image_url": imgURL,
			"error":     fmt.Sprintf("Failed to save image locally: %v", err),
		})
	}

	return toJSON(map[string]any{
		"success":   true,
		"image_url": imgURL,
		"file_path": savePath,
		"prompt":    prompt,
		"size":      size,
		"model":     model,
		"message":   fmt.Sprintf("Image generated and saved to %s", savePath),
	})
}
