package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/hermes-agent/hermes-agent-go/internal/config"
)

// allowlistedEnvKeys are API keys that are safe to migrate.
var allowlistedEnvKeys = map[string]bool{
	"OPENROUTER_API_KEY": true,
	"OPENAI_API_KEY":     true,
	"ANTHROPIC_API_KEY":  true,
	"DEEPSEEK_API_KEY":   true,
	"GOOGLE_API_KEY":     true,
	"NOUS_API_KEY":       true,
	"EXA_API_KEY":        true,
	"FIRECRAWL_API_KEY":  true,
}

// RunClawMigrate migrates configuration and data from ~/.openclaw to ~/.hermes.
// When dryRun is true, it only reports what would be done without making changes.
// When overwrite is true, it replaces existing files in the target.
func RunClawMigrate(dryRun, overwrite bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}

	sourceDir := filepath.Join(home, ".openclaw")
	targetDir := config.HermesHome()

	// Verify source exists.
	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		fmt.Println("No OpenClaw installation found at ~/.openclaw")
		fmt.Println("Nothing to migrate.")
		return nil
	}

	if dryRun {
		fmt.Println("=== DRY RUN (no changes will be made) ===")
	}

	fmt.Printf("Migrating from %s to %s\n\n", sourceDir, targetDir)

	var migrated, skipped, errors int

	// 1. Migrate SOUL.md (persona)
	m, s, e := migrateFile(sourceDir, targetDir, "SOUL.md", "", dryRun, overwrite)
	migrated += m
	skipped += s
	errors += e

	// 2. Migrate MEMORY.md
	m, s, e = migrateFile(sourceDir, targetDir, "MEMORY.md", filepath.Join("memories", "MEMORY.md"), dryRun, overwrite)
	migrated += m
	skipped += s
	errors += e

	// 3. Migrate USER.md
	m, s, e = migrateFile(sourceDir, targetDir, "USER.md", filepath.Join("memories", "USER.md"), dryRun, overwrite)
	migrated += m
	skipped += s
	errors += e

	// 4. Migrate skills
	m, s, e = migrateSkills(sourceDir, targetDir, dryRun, overwrite)
	migrated += m
	skipped += s
	errors += e

	// 5. Migrate API keys (selective, allowlisted)
	m, s, e = migrateEnvKeys(sourceDir, targetDir, dryRun, overwrite)
	migrated += m
	skipped += s
	errors += e

	// 6. Migrate command allowlist
	m, s, e = migrateFile(sourceDir, targetDir, "command_allowlist.txt", "command_allowlist.txt", dryRun, overwrite)
	migrated += m
	skipped += s
	errors += e

	// 7. Migrate messaging settings from config
	m, s, e = migrateMessagingSettings(sourceDir, targetDir, dryRun, overwrite)
	migrated += m
	skipped += s
	errors += e

	// Summary
	fmt.Println()
	fmt.Println("Migration Summary")
	fmt.Println("=================")
	fmt.Printf("  Migrated: %d\n", migrated)
	fmt.Printf("  Skipped:  %d (already exist)\n", skipped)
	fmt.Printf("  Errors:   %d\n", errors)

	if dryRun {
		fmt.Println()
		fmt.Println("Run without --dry-run to apply changes.")
	}

	return nil
}

// migrateFile copies a single file from source to target.
// If relTarget is empty, uses the same relative path as the source file.
// Returns (migrated, skipped, errors) counts.
func migrateFile(sourceDir, targetDir, relSource, relTarget string, dryRun, overwrite bool) (int, int, int) {
	if relTarget == "" {
		relTarget = relSource
	}

	srcPath := filepath.Join(sourceDir, relSource)
	dstPath := filepath.Join(targetDir, relTarget)

	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		return 0, 0, 0 // Source doesn't exist, silently skip.
	}

	if !overwrite {
		if _, err := os.Stat(dstPath); err == nil {
			fmt.Printf("  SKIP  %s (already exists)\n", relTarget)
			return 0, 1, 0
		}
	}

	if dryRun {
		fmt.Printf("  COPY  %s -> %s\n", relSource, relTarget)
		return 1, 0, 0
	}

	// Ensure target directory exists.
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		fmt.Printf("  ERROR creating directory for %s: %v\n", relTarget, err)
		return 0, 0, 1
	}

	if err := copyFile(srcPath, dstPath); err != nil {
		fmt.Printf("  ERROR %s: %v\n", relTarget, err)
		return 0, 0, 1
	}

	fmt.Printf("  OK    %s\n", relTarget)
	return 1, 0, 0
}

// migrateSkills copies skill directories from the OpenClaw skills folder
// into ~/.hermes/skills/openclaw-imports/.
func migrateSkills(sourceDir, targetDir string, dryRun, overwrite bool) (int, int, int) {
	srcSkills := filepath.Join(sourceDir, "skills")
	if _, err := os.Stat(srcSkills); os.IsNotExist(err) {
		return 0, 0, 0
	}

	dstSkills := filepath.Join(targetDir, "skills", "openclaw-imports")
	var migrated, skipped, errors int

	entries, err := os.ReadDir(srcSkills)
	if err != nil {
		fmt.Printf("  ERROR reading skills directory: %v\n", err)
		return 0, 0, 1
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		srcSkillDir := filepath.Join(srcSkills, entry.Name())
		dstSkillDir := filepath.Join(dstSkills, entry.Name())

		if !overwrite {
			if _, err := os.Stat(dstSkillDir); err == nil {
				fmt.Printf("  SKIP  skills/openclaw-imports/%s (already exists)\n", entry.Name())
				skipped++
				continue
			}
		}

		if dryRun {
			fmt.Printf("  COPY  skills/%s -> skills/openclaw-imports/%s\n", entry.Name(), entry.Name())
			migrated++
			continue
		}

		if err := copyDir(srcSkillDir, dstSkillDir); err != nil {
			fmt.Printf("  ERROR skills/%s: %v\n", entry.Name(), err)
			errors++
			continue
		}

		fmt.Printf("  OK    skills/openclaw-imports/%s\n", entry.Name())
		migrated++
	}

	return migrated, skipped, errors
}

// migrateEnvKeys reads the .env file from the source directory and copies
// only allowlisted API keys to the target .env file.
func migrateEnvKeys(sourceDir, targetDir string, dryRun, overwrite bool) (int, int, int) {
	srcEnv := filepath.Join(sourceDir, ".env")
	if _, err := os.Stat(srcEnv); os.IsNotExist(err) {
		return 0, 0, 0
	}

	data, err := os.ReadFile(srcEnv)
	if err != nil {
		fmt.Printf("  ERROR reading source .env: %v\n", err)
		return 0, 0, 1
	}

	dstEnvPath := filepath.Join(targetDir, ".env")
	existingLines := loadExistingEnvLines(dstEnvPath)

	var migrated, skipped int

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimPrefix(strings.TrimSpace(parts[0]), "export ")
		value := strings.TrimSpace(parts[1])

		if !allowlistedEnvKeys[key] {
			continue // Skip non-allowlisted keys.
		}

		if value == "" {
			continue
		}

		// Check if already set in target.
		if !overwrite && envKeyExists(existingLines, key) {
			fmt.Printf("  SKIP  env key %s (already set)\n", key)
			skipped++
			continue
		}

		if dryRun {
			fmt.Printf("  COPY  env key %s\n", key)
			migrated++
			continue
		}

		existingLines = setEnvLine(existingLines, key, value)
		fmt.Printf("  OK    env key %s\n", key)
		migrated++
	}

	if !dryRun && migrated > 0 {
		content := strings.Join(existingLines, "\n") + "\n"
		if err := os.WriteFile(dstEnvPath, []byte(content), 0600); err != nil {
			fmt.Printf("  ERROR saving .env: %v\n", err)
			return 0, 0, 1
		}
	}

	return migrated, skipped, 0
}

// migrateMessagingSettings copies messaging-related env vars (bot tokens).
func migrateMessagingSettings(sourceDir, targetDir string, dryRun, overwrite bool) (int, int, int) {
	srcEnv := filepath.Join(sourceDir, ".env")
	if _, err := os.Stat(srcEnv); os.IsNotExist(err) {
		return 0, 0, 0
	}

	data, err := os.ReadFile(srcEnv)
	if err != nil {
		return 0, 0, 0
	}

	messagingKeys := map[string]bool{
		"TELEGRAM_BOT_TOKEN": true,
		"DISCORD_BOT_TOKEN":  true,
		"SLACK_BOT_TOKEN":    true,
		"SLACK_APP_TOKEN":    true,
	}

	dstEnvPath := filepath.Join(targetDir, ".env")
	existingLines := loadExistingEnvLines(dstEnvPath)

	var migrated, skipped int

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimPrefix(strings.TrimSpace(parts[0]), "export ")
		value := strings.TrimSpace(parts[1])

		if !messagingKeys[key] || value == "" {
			continue
		}

		if !overwrite && envKeyExists(existingLines, key) {
			fmt.Printf("  SKIP  messaging key %s (already set)\n", key)
			skipped++
			continue
		}

		if dryRun {
			fmt.Printf("  COPY  messaging key %s\n", key)
			migrated++
			continue
		}

		existingLines = setEnvLine(existingLines, key, value)
		fmt.Printf("  OK    messaging key %s\n", key)
		migrated++
	}

	if !dryRun && migrated > 0 {
		content := strings.Join(existingLines, "\n") + "\n"
		if err := os.WriteFile(dstEnvPath, []byte(content), 0600); err != nil {
			fmt.Printf("  ERROR saving .env: %v\n", err)
			return 0, 0, 1
		}
	}

	return migrated, skipped, 0
}

// --- file helpers ---

func envKeyExists(lines []string, key string) bool {
	prefix := key + "="
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, prefix) || strings.HasPrefix(trimmed, "export "+prefix) {
			return true
		}
	}
	return false
}

func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		return copyFile(path, dstPath)
	})
}
