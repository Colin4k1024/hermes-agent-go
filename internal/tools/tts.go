package tools

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/config"
)

func init() {
	Register(&ToolEntry{
		Name:    "text_to_speech",
		Toolset: "tts",
		Schema: map[string]any{
			"name":        "text_to_speech",
			"description": "Convert text to speech audio using edge-tts. Returns the path to the generated audio file.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"text": map[string]any{
						"type":        "string",
						"description": "Text to convert to speech",
					},
					"voice": map[string]any{
						"type":        "string",
						"description": "Voice to use (default: zh-CN-XiaoxiaoNeural). Run 'edge-tts --list-voices' for options.",
						"default":     "zh-CN-XiaoxiaoNeural",
					},
					"rate": map[string]any{
						"type":        "string",
						"description": "Speech rate adjustment (e.g., '+20%', '-10%')",
					},
					"volume": map[string]any{
						"type":        "string",
						"description": "Volume adjustment (e.g., '+50%', '-20%')",
					},
					"output_format": map[string]any{
						"type":        "string",
						"description": "Output audio format: 'mp3' or 'wav'",
						"default":     "mp3",
					},
				},
				"required": []string{"text"},
			},
		},
		Handler: handleTextToSpeech,
		CheckFn: checkTTSRequirements,
		Emoji:   "\U0001f50a",
	})
}

func checkTTSRequirements() bool {
	_, err := exec.LookPath("edge-tts")
	return err == nil
}

func handleTextToSpeech(args map[string]any, ctx *ToolContext) string {
	text, _ := args["text"].(string)
	if text == "" {
		return `{"error":"text is required"}`
	}

	voice, _ := args["voice"].(string)
	if voice == "" {
		voice = "zh-CN-XiaoxiaoNeural"
	}

	rate, _ := args["rate"].(string)
	volume, _ := args["volume"].(string)

	outputFormat, _ := args["output_format"].(string)
	if outputFormat == "" {
		outputFormat = "mp3"
	}
	if outputFormat != "mp3" && outputFormat != "wav" {
		outputFormat = "mp3"
	}

	// Ensure audio cache directory exists
	audioDir := filepath.Join(config.HermesHome(), "cache", "audio")
	os.MkdirAll(audioDir, 0755)

	// Generate output filename
	filename := fmt.Sprintf("tts_%d.%s", time.Now().UnixMilli(), outputFormat)
	outputPath := filepath.Join(audioDir, filename)

	// Build edge-tts command
	cmdArgs := []string{
		"--voice", voice,
		"--text", text,
		"--write-media", outputPath,
	}

	if rate != "" {
		cmdArgs = append(cmdArgs, "--rate", rate)
	}
	if volume != "" {
		cmdArgs = append(cmdArgs, "--volume", volume)
	}

	execCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "edge-tts", cmdArgs...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	startTime := time.Now()
	err := cmd.Run()
	duration := time.Since(startTime)

	if err != nil {
		errMsg := stderr.String()
		if execCtx.Err() == context.DeadlineExceeded {
			errMsg = "TTS generation timed out"
		}
		return toJSON(map[string]any{
			"error":   fmt.Sprintf("TTS failed: %v", err),
			"stderr":  truncateOutput(errMsg, 500),
			"hint":    "Ensure edge-tts is installed: pip install edge-tts",
		})
	}

	// Verify output file exists
	info, err := os.Stat(outputPath)
	if err != nil {
		return toJSON(map[string]any{"error": "Audio file was not created"})
	}

	return toJSON(map[string]any{
		"success":     true,
		"file_path":   outputPath,
		"voice":       voice,
		"format":      outputFormat,
		"size_bytes":  info.Size(),
		"duration_ms": duration.Milliseconds(),
		"text_length": len(text),
		"message":     fmt.Sprintf("Audio saved to %s", outputPath),
	})
}

// ListTTSVoices returns available edge-tts voices (utility function).
func ListTTSVoices(filter string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "edge-tts", "--list-voices")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to list voices: %w", err)
	}

	output := stdout.String()
	if filter != "" {
		var filtered []string
		for _, line := range strings.Split(output, "\n") {
			if strings.Contains(strings.ToLower(line), strings.ToLower(filter)) {
				filtered = append(filtered, line)
			}
		}
		return strings.Join(filtered, "\n"), nil
	}

	return output, nil
}
