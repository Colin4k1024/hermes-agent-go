package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// VoiceMode manages speech-to-text and text-to-speech for voice interaction.
type VoiceMode struct {
	Enabled   bool
	STTEngine string // "whisper" (local), "openai" (API)
	TTSEngine string // "edge-tts" (free), "elevenlabs", "openai"
	AutoTTS   bool   // automatically speak responses
}

// NewVoiceMode creates a VoiceMode with sensible defaults.
func NewVoiceMode() *VoiceMode {
	return &VoiceMode{
		Enabled:   false,
		STTEngine: "whisper",
		TTSEngine: "edge-tts",
		AutoTTS:   false,
	}
}

// TranscribeAudio transcribes an audio file to text using the configured STT engine.
// Supports "whisper" (local CLI) and "openai" (Whisper API).
func (v *VoiceMode) TranscribeAudio(audioPath string) (string, error) {
	if _, err := os.Stat(audioPath); os.IsNotExist(err) {
		return "", fmt.Errorf("audio file not found: %s", audioPath)
	}

	switch strings.ToLower(v.STTEngine) {
	case "openai":
		return v.transcribeOpenAI(audioPath)
	case "whisper":
		return v.transcribeWhisper(audioPath)
	default:
		return v.transcribeWhisper(audioPath)
	}
}

// SpeakText converts text to speech audio using the configured TTS engine.
// Returns the path to the generated audio file.
func (v *VoiceMode) SpeakText(text string) (string, error) {
	if text == "" {
		return "", fmt.Errorf("text is required for TTS")
	}

	switch strings.ToLower(v.TTSEngine) {
	case "edge-tts":
		return v.speakEdgeTTS(text)
	case "openai":
		return v.speakOpenAI(text)
	case "elevenlabs":
		return v.speakElevenLabs(text)
	default:
		return v.speakEdgeTTS(text)
	}
}

// IsAvailable checks if the required tools for voice mode are installed.
func (v *VoiceMode) IsAvailable() bool {
	// Check STT availability.
	sttAvailable := false
	switch strings.ToLower(v.STTEngine) {
	case "openai":
		sttAvailable = os.Getenv("OPENAI_API_KEY") != ""
	case "whisper":
		_, err := exec.LookPath("whisper")
		sttAvailable = err == nil
	}

	// Check TTS availability.
	ttsAvailable := false
	switch strings.ToLower(v.TTSEngine) {
	case "edge-tts":
		_, err := exec.LookPath("edge-tts")
		ttsAvailable = err == nil
	case "openai":
		ttsAvailable = os.Getenv("OPENAI_API_KEY") != ""
	case "elevenlabs":
		ttsAvailable = os.Getenv("ELEVENLABS_API_KEY") != ""
	}

	return sttAvailable || ttsAvailable
}

// STTAvailable returns true if the STT engine is available.
func (v *VoiceMode) STTAvailable() bool {
	switch strings.ToLower(v.STTEngine) {
	case "openai":
		return os.Getenv("OPENAI_API_KEY") != ""
	case "whisper":
		_, err := exec.LookPath("whisper")
		return err == nil
	}
	return false
}

// TTSAvailable returns true if the TTS engine is available.
func (v *VoiceMode) TTSAvailable() bool {
	switch strings.ToLower(v.TTSEngine) {
	case "edge-tts":
		_, err := exec.LookPath("edge-tts")
		return err == nil
	case "openai":
		return os.Getenv("OPENAI_API_KEY") != ""
	case "elevenlabs":
		return os.Getenv("ELEVENLABS_API_KEY") != ""
	}
	return false
}

// Status returns a human-readable status of voice mode.
func (v *VoiceMode) Status() string {
	enabledStr := "disabled"
	if v.Enabled {
		enabledStr = "enabled"
	}

	sttStatus := "unavailable"
	if v.STTAvailable() {
		sttStatus = "available"
	}

	ttsStatus := "unavailable"
	if v.TTSAvailable() {
		ttsStatus = "available"
	}

	return fmt.Sprintf("Voice mode: %s\nSTT engine: %s (%s)\nTTS engine: %s (%s)\nAuto-TTS: %v",
		enabledStr, v.STTEngine, sttStatus, v.TTSEngine, ttsStatus, v.AutoTTS)
}

// --- Internal STT implementations ---

func (v *VoiceMode) transcribeWhisper(audioPath string) (string, error) {
	whisperPath, err := exec.LookPath("whisper")
	if err != nil {
		return "", fmt.Errorf("whisper CLI not found. Install it with: pip install openai-whisper")
	}

	// Create a temp directory for output.
	tmpDir, err := os.MkdirTemp("", "hermes-whisper-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	cmd := exec.Command(whisperPath, audioPath,
		"--model", "base",
		"--output_format", "txt",
		"--output_dir", tmpDir,
	)
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("whisper transcription failed: %w", err)
	}

	// Read the output .txt file.
	base := strings.TrimSuffix(filepath.Base(audioPath), filepath.Ext(audioPath))
	outputPath := filepath.Join(tmpDir, base+".txt")

	data, err := os.ReadFile(outputPath)
	if err != nil {
		return "", fmt.Errorf("read transcription output: %w", err)
	}

	return strings.TrimSpace(string(data)), nil
}

func (v *VoiceMode) transcribeOpenAI(audioPath string) (string, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("OPENAI_API_KEY is required for OpenAI Whisper STT")
	}

	// Use curl as a simple HTTP client for multipart upload.
	curlPath, err := exec.LookPath("curl")
	if err != nil {
		return "", fmt.Errorf("curl not found, required for OpenAI Whisper API")
	}

	cmd := exec.Command(curlPath,
		"-s",
		"https://api.openai.com/v1/audio/transcriptions",
		"-H", "Authorization: Bearer "+apiKey,
		"-F", "file=@"+audioPath,
		"-F", "model=whisper-1",
		"-F", "response_format=text",
	)

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("OpenAI Whisper API call failed: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// --- Internal TTS implementations ---

func (v *VoiceMode) speakEdgeTTS(text string) (string, error) {
	edgeTTSPath, err := exec.LookPath("edge-tts")
	if err != nil {
		return "", fmt.Errorf("edge-tts not found. Install it with: pip install edge-tts")
	}

	// Generate output path.
	outputPath := filepath.Join(os.TempDir(),
		fmt.Sprintf("hermes-tts-%d.mp3", time.Now().UnixNano()))

	cmd := exec.Command(edgeTTSPath,
		"--text", text,
		"--write-media", outputPath,
	)
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("edge-tts failed: %w", err)
	}

	return outputPath, nil
}

func (v *VoiceMode) speakOpenAI(text string) (string, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("OPENAI_API_KEY is required for OpenAI TTS")
	}

	outputPath := filepath.Join(os.TempDir(),
		fmt.Sprintf("hermes-tts-%d.mp3", time.Now().UnixNano()))

	curlPath, err := exec.LookPath("curl")
	if err != nil {
		return "", fmt.Errorf("curl not found, required for OpenAI TTS API")
	}

	cmd := exec.Command(curlPath,
		"-s",
		"https://api.openai.com/v1/audio/speech",
		"-H", "Authorization: Bearer "+apiKey,
		"-H", "Content-Type: application/json",
		"-d", fmt.Sprintf(`{"model":"tts-1","input":%q,"voice":"alloy"}`, text),
		"--output", outputPath,
	)

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("OpenAI TTS API call failed: %w", err)
	}

	return outputPath, nil
}

func (v *VoiceMode) speakElevenLabs(text string) (string, error) {
	apiKey := os.Getenv("ELEVENLABS_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("ELEVENLABS_API_KEY is required for ElevenLabs TTS")
	}

	outputPath := filepath.Join(os.TempDir(),
		fmt.Sprintf("hermes-tts-%d.mp3", time.Now().UnixNano()))

	curlPath, err := exec.LookPath("curl")
	if err != nil {
		return "", fmt.Errorf("curl not found, required for ElevenLabs TTS API")
	}

	// Use Rachel voice as default.
	voiceID := "21m00Tcm4TlvDq8ikWAM"

	cmd := exec.Command(curlPath,
		"-s",
		fmt.Sprintf("https://api.elevenlabs.io/v1/text-to-speech/%s", voiceID),
		"-H", "xi-api-key: "+apiKey,
		"-H", "Content-Type: application/json",
		"-d", fmt.Sprintf(`{"text":%q,"model_id":"eleven_monolingual_v1"}`, text),
		"--output", outputPath,
	)

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ElevenLabs TTS API call failed: %w", err)
	}

	return outputPath, nil
}
