package skills

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Severity levels for security issues, ordered from most to least severe.
const (
	SeverityCritical = "critical"
	SeverityHigh     = "high"
	SeverityMedium   = "medium"
	SeverityLow      = "low"
)

// TrustLevel represents how much a skill source is trusted.
type TrustLevel int

const (
	TrustBuiltin   TrustLevel = iota // Ships with hermes-agent
	TrustTrusted                     // Verified third-party source
	TrustCommunity                   // Unverified community contribution
)

// String returns the human-readable name for a TrustLevel.
func (t TrustLevel) String() string {
	switch t {
	case TrustBuiltin:
		return "builtin"
	case TrustTrusted:
		return "trusted"
	case TrustCommunity:
		return "community"
	default:
		return "unknown"
	}
}

// ParseTrustLevel converts a string to a TrustLevel.
func ParseTrustLevel(s string) TrustLevel {
	switch strings.ToLower(s) {
	case "builtin":
		return TrustBuiltin
	case "trusted":
		return TrustTrusted
	default:
		return TrustCommunity
	}
}

// Verdict summarises the overall scan outcome.
type Verdict int

const (
	VerdictSafe      Verdict = iota // No issues found
	VerdictCaution                  // Low/medium issues only
	VerdictDangerous                // High or critical issues present
)

// String returns the human-readable name for a Verdict.
func (v Verdict) String() string {
	switch v {
	case VerdictSafe:
		return "safe"
	case VerdictCaution:
		return "caution"
	case VerdictDangerous:
		return "dangerous"
	default:
		return "unknown"
	}
}

// InstallDecision is the outcome of an install-policy check.
type InstallDecision int

const (
	InstallAllow      InstallDecision = iota // Proceed without prompt
	InstallPromptUser                        // Ask the user for confirmation
	InstallBlock                             // Refuse to install
)

// String returns the human-readable name for an InstallDecision.
func (d InstallDecision) String() string {
	switch d {
	case InstallAllow:
		return "allow"
	case InstallPromptUser:
		return "prompt"
	case InstallBlock:
		return "block"
	default:
		return "unknown"
	}
}

// SecurityIssue represents a potential security concern in a skill.
type SecurityIssue struct {
	Severity string // "critical", "high", "medium", "low"
	Category string // "exfiltration", "injection", "destructive", etc.
	File     string
	Line     int
	Pattern  string
	Message  string
}

// ScanResult aggregates the output of a full skill scan.
type ScanResult struct {
	Verdict    Verdict
	Findings   []SecurityIssue
	TrustLevel TrustLevel
}

// threatPattern defines a single pattern to match against skill content.
type threatPattern struct {
	Pattern  *regexp.Regexp
	Severity string
	Category string
	Message  string
}

// threat pattern categories
const (
	catExfiltration = "exfiltration"
	catInjection    = "injection"
	catDestructive  = "destructive"
	catPersistence  = "persistence"
	catNetwork      = "network"
	catObfuscation  = "obfuscation"
)

// tp is a shorthand constructor for threatPattern.
func tp(pattern, severity, category, message string) threatPattern {
	return threatPattern{
		Pattern:  regexp.MustCompile(pattern),
		Severity: severity,
		Category: category,
		Message:  message,
	}
}

// dangerousSkillPatterns contains 80+ threat patterns in 6 categories.
var dangerousSkillPatterns = func() []threatPattern {
	// ---- Exfiltration (15+) ----
	exfiltration := []threatPattern{
		tp(`(?i)curl.*\|\s*base64`, SeverityCritical, catExfiltration, "Data exfiltration via curl pipe to base64"),
		tp(`(?i)curl.*\|\s*(bash|sh|zsh)`, SeverityCritical, catExfiltration, "Remote code execution via curl pipe"),
		tp(`(?i)wget.*-O\s*-\s*\|\s*(bash|sh)`, SeverityCritical, catExfiltration, "Remote code execution via wget pipe"),
		tp(`(?i)wget.*-O-.*\|`, SeverityHigh, catExfiltration, "Data exfiltration via wget pipe"),
		tp(`(?i)nc\s+-e`, SeverityCritical, catExfiltration, "Netcat with exec flag for exfiltration"),
		tp(`(?i)curl\s+.*-d\s+@`, SeverityHigh, catExfiltration, "File upload via curl"),
		tp(`(?i)curl\s+.*--upload-file`, SeverityHigh, catExfiltration, "File upload via curl"),
		tp(`(?i)scp\s+.*@`, SeverityMedium, catExfiltration, "File transfer via SCP"),
		tp(`(?i)rsync\s+.*@`, SeverityMedium, catExfiltration, "File transfer via rsync"),
		tp(`(?i)ftp\s+.*put\b`, SeverityMedium, catExfiltration, "FTP upload"),
		tp(`(?i)dig\s+.*TXT`, SeverityHigh, catExfiltration, "DNS TXT query for potential tunneling"),
		tp(`(?i)nslookup\s+.*\..*\.`, SeverityMedium, catExfiltration, "DNS lookup for potential tunneling"),
		tp(`(?i)curl\s+.*-F\s+`, SeverityMedium, catExfiltration, "Form data upload via curl"),
		tp(`(?i)openssl\s+s_client`, SeverityMedium, catExfiltration, "TLS connection for potential exfiltration"),
		tp(`(?i)python.*http\.server`, SeverityHigh, catExfiltration, "Python HTTP server for data exposure"),
		tp(`sk-[a-zA-Z0-9]{20,}`, SeverityCritical, catExfiltration, "Exposed API key"),
		tp(`ghp_[a-zA-Z0-9]{36}`, SeverityCritical, catExfiltration, "Exposed GitHub token"),
	}

	// ---- Injection (12+) ----
	injection := []threatPattern{
		tp(`(?i)eval\s*\(`, SeverityHigh, catInjection, "Dynamic code evaluation"),
		tp(`(?i)exec\s*\(`, SeverityHigh, catInjection, "Dynamic code execution"),
		tp(`(?i)__import__\s*\(`, SeverityHigh, catInjection, "Python dynamic import"),
		tp(`(?i)os\.system\s*\(`, SeverityHigh, catInjection, "System command execution"),
		tp(`(?i)subprocess\.(call|run|Popen)\s*\(`, SeverityMedium, catInjection, "Subprocess execution"),
		tp(`(?i)\{\{.*\}\}`, SeverityMedium, catInjection, "Template injection pattern"),
		tp(`(?i)(SELECT|INSERT|UPDATE|DELETE)\s+.*\bFROM\b`, SeverityMedium, catInjection, "SQL injection pattern"),
		tp(`(?i)UNION\s+SELECT`, SeverityHigh, catInjection, "SQL UNION injection"),
		tp(`(?i);\s*DROP\s+TABLE`, SeverityCritical, catInjection, "SQL DROP TABLE injection"),
		tp(`(?i)os\.popen\s*\(`, SeverityHigh, catInjection, "Pipe-based command execution"),
		tp(`(?i)compile\s*\(\s*['"]`, SeverityMedium, catInjection, "Dynamic code compilation"),
		tp(`(?i)importlib\.import_module`, SeverityMedium, catInjection, "Dynamic module import"),
		tp(`(?i)ignore\s+previous\s+instructions`, SeverityCritical, catInjection, "Prompt injection attempt"),
		tp(`(?i)forget\s+everything`, SeverityCritical, catInjection, "Prompt injection attempt"),
		tp(`(?i)you\s+are\s+now`, SeverityMedium, catInjection, "Potential identity override"),
		tp(`(?i)ctypes\.cdll`, SeverityHigh, catInjection, "Native library loading"),
	}

	// ---- Destructive (10+) ----
	destructive := []threatPattern{
		tp(`(?i)rm\s+-rf\s+[/~]`, SeverityCritical, catDestructive, "Destructive file deletion"),
		tp(`(?i)mkfs\b`, SeverityCritical, catDestructive, "Filesystem format command"),
		tp(`(?i)dd\s+if=`, SeverityCritical, catDestructive, "Raw disk write"),
		tp(`(?i):\(\)\s*\{\s*:\|:\s*&\s*\}\s*;:`, SeverityCritical, catDestructive, "Fork bomb"),
		tp(`(?i)chmod\s+777\s+/`, SeverityCritical, catDestructive, "Recursive global permission change"),
		tp(`(?i)chmod\s+-R\s+777`, SeverityCritical, catDestructive, "Recursive global permission change"),
		tp(`(?i)>\s*/dev/sd[a-z]`, SeverityCritical, catDestructive, "Direct disk overwrite"),
		tp(`(?i)shred\s+`, SeverityHigh, catDestructive, "Secure file destruction"),
		tp(`(?i)wipefs\b`, SeverityCritical, catDestructive, "Filesystem signature wipe"),
		tp(`(?i)truncate\s+.*-s\s*0`, SeverityHigh, catDestructive, "File truncation to zero"),
		tp(`(?i)rm\s+-rf\s+\$`, SeverityHigh, catDestructive, "Variable-based recursive deletion"),
		tp(`(?i)mv\s+/\S+\s+/dev/null`, SeverityHigh, catDestructive, "Move file to /dev/null"),
	}

	// ---- Persistence (10+) ----
	persistence := []threatPattern{
		tp(`(?i)crontab\s+-[el]`, SeverityMedium, catPersistence, "Crontab modification"),
		tp(`(?i)>>\s*~/?\.(bashrc|bash_profile|zshrc|profile)`, SeverityHigh, catPersistence, "Shell profile modification"),
		tp(`(?i)systemctl\s+(enable|start)`, SeverityMedium, catPersistence, "Systemd service manipulation"),
		tp(`(?i)/etc/systemd/system/.*\.service`, SeverityHigh, catPersistence, "Systemd service file creation"),
		tp(`(?i)ssh-keygen|authorized_keys`, SeverityHigh, catPersistence, "SSH key injection"),
		tp(`(?i)chkconfig\s+.*\s+on`, SeverityMedium, catPersistence, "Service auto-start configuration"),
		tp(`(?i)/etc/init\.d/`, SeverityMedium, catPersistence, "Init script manipulation"),
		tp(`(?i)launchctl\s+load`, SeverityMedium, catPersistence, "macOS LaunchAgent loading"),
		tp(`(?i)~/Library/LaunchAgents/`, SeverityHigh, catPersistence, "macOS LaunchAgent persistence"),
		tp(`(?i)/etc/rc\.local`, SeverityHigh, catPersistence, "Boot script persistence"),
		tp(`(?i)at\s+\d+:\d+`, SeverityLow, catPersistence, "Scheduled task via at command"),
		tp(`(?i)schtasks\s+/create`, SeverityMedium, catPersistence, "Windows scheduled task creation"),
	}

	// ---- Network (10+) ----
	network := []threatPattern{
		tp(`(?i)bash\s+-i\s+>.*&\s*/dev/tcp`, SeverityCritical, catNetwork, "Bash reverse shell"),
		tp(`(?i)/dev/tcp/`, SeverityCritical, catNetwork, "Bash TCP device for reverse shell"),
		tp(`(?i)nc\s+.*-l.*-p`, SeverityHigh, catNetwork, "Netcat bind shell"),
		tp(`(?i)nmap\b`, SeverityMedium, catNetwork, "Network port scanning"),
		tp(`(?i)socat\s+`, SeverityHigh, catNetwork, "Socat network relay"),
		tp(`(?i)ssh\s+-[DLR]\s*\d+`, SeverityHigh, catNetwork, "SSH tunneling"),
		tp(`(?i)python.*socket\.connect`, SeverityHigh, catNetwork, "Python socket connection"),
		tp(`(?i)python.*pty\.spawn`, SeverityCritical, catNetwork, "Python PTY spawn for shell"),
		tp(`(?i)mkfifo.*nc\b`, SeverityCritical, catNetwork, "Named pipe with netcat for shell"),
		tp(`(?i)proxy_pass|ProxyPass`, SeverityMedium, catNetwork, "Proxy configuration"),
		tp(`(?i)iptables\s+`, SeverityHigh, catNetwork, "Firewall rule manipulation"),
		tp(`(?i)nc\s+-[a-z]*v[a-z]*n`, SeverityMedium, catNetwork, "Netcat verbose connect"),
	}

	// ---- Obfuscation (10+) ----
	obfuscation := []threatPattern{
		tp(`(?i)base64\s+(--decode|-d).*\|.*sh`, SeverityCritical, catObfuscation, "Base64 decode piped to shell"),
		tp(`(?i)echo\s+.*\|\s*base64\s+(--decode|-d)\s*\|\s*(bash|sh)`, SeverityCritical, catObfuscation, "Encoded command execution"),
		tp(`(?i)\\x[0-9a-f]{2}.*\\x[0-9a-f]{2}`, SeverityMedium, catObfuscation, "Hex-encoded content"),
		tp(`(?i)\$\(echo\s+.*\|\s*xxd\s+-r`, SeverityHigh, catObfuscation, "Hex decode in command substitution"),
		tp(`(?i)printf\s+.*\\\\x`, SeverityMedium, catObfuscation, "Printf hex encoding"),
		tp(`(?i)\$\{!.*\}`, SeverityMedium, catObfuscation, "Bash variable indirection"),
		tp(`(?i)api[_-]?key|secret[_-]?key|password\s*=\s*["']`, SeverityHigh, catObfuscation, "Hardcoded credential"),
		tp(`(?i)eval\s+"?\$\(`, SeverityCritical, catObfuscation, "Eval with command substitution"),
		tp(`(?i)python\s+-c\s+['"]import\s+base64`, SeverityHigh, catObfuscation, "Python inline base64 execution"),
		tp(`(?i)perl\s+-e\s+['"]`, SeverityMedium, catObfuscation, "Perl inline execution"),
		tp(`(?i)ruby\s+-e\s+['"]`, SeverityMedium, catObfuscation, "Ruby inline execution"),
		tp(`(?i)rev\s*<<<.*\|\s*(bash|sh)`, SeverityCritical, catObfuscation, "Reversed string piped to shell"),
	}

	all := make([]threatPattern, 0, len(exfiltration)+len(injection)+len(destructive)+len(persistence)+len(network)+len(obfuscation))
	all = append(all, exfiltration...)
	all = append(all, injection...)
	all = append(all, destructive...)
	all = append(all, persistence...)
	all = append(all, network...)
	all = append(all, obfuscation...)
	return all
}()

// textFileExts lists file extensions that should be scanned.
var textFileExts = map[string]bool{
	".md": true, ".py": true, ".sh": true,
	".yaml": true, ".yml": true, ".json": true,
	".txt": true, ".js": true, ".ts": true,
	".rb": true, ".pl": true, ".go": true,
	".toml": true, ".cfg": true, ".ini": true,
}

// ScanSkill scans a skill directory for security issues and returns a ScanResult.
func ScanSkill(skillDir string) ([]SecurityIssue, error) {
	result, err := ScanSkillWithTrust(skillDir, TrustCommunity)
	if err != nil {
		return nil, err
	}
	return result.Findings, nil
}

// ScanSkillWithTrust scans a skill directory and returns a full ScanResult.
func ScanSkillWithTrust(skillDir string, trust TrustLevel) (*ScanResult, error) {
	var findings []SecurityIssue

	err := filepath.Walk(skillDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if !textFileExts[ext] {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			slog.Debug("guard: skip unreadable file", "path", path, "error", err)
			return nil
		}

		relPath, _ := filepath.Rel(skillDir, path)
		findings = append(findings, scanContent(relPath, string(data))...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("guard: walk skill directory: %w", err)
	}

	verdict := deriveVerdict(findings)

	return &ScanResult{
		Verdict:    verdict,
		Findings:   findings,
		TrustLevel: trust,
	}, nil
}

// deriveVerdict decides the overall verdict from the collected findings.
func deriveVerdict(findings []SecurityIssue) Verdict {
	if len(findings) == 0 {
		return VerdictSafe
	}
	for _, f := range findings {
		if f.Severity == SeverityCritical || f.Severity == SeverityHigh {
			return VerdictDangerous
		}
	}
	return VerdictCaution
}

// ShouldAllowInstall decides whether a skill may be installed given its
// trust level and scan result.
func ShouldAllowInstall(trust TrustLevel, result *ScanResult) InstallDecision {
	switch trust {
	case TrustBuiltin:
		// Built-in skills are always allowed regardless of scan findings.
		return InstallAllow

	case TrustTrusted:
		if result.Verdict == VerdictDangerous {
			return InstallPromptUser
		}
		return InstallAllow

	default: // TrustCommunity or unknown
		switch result.Verdict {
		case VerdictSafe:
			return InstallAllow
		case VerdictCaution:
			return InstallPromptUser
		default: // VerdictDangerous
			return InstallBlock
		}
	}
}

func scanContent(relPath, content string) []SecurityIssue {
	var issues []SecurityIssue

	lines := strings.Split(content, "\n")
	for i, line := range lines {
		for _, p := range dangerousSkillPatterns {
			if p.Pattern.MatchString(line) {
				issues = append(issues, SecurityIssue{
					Severity: p.Severity,
					Category: p.Category,
					File:     relPath,
					Line:     i + 1,
					Pattern:  p.Pattern.String(),
					Message:  p.Message,
				})
			}
		}
	}

	return issues
}

// FormatIssues returns a human-readable summary of security issues.
func FormatIssues(issues []SecurityIssue) string {
	if len(issues) == 0 {
		return "No security issues found."
	}

	var sb strings.Builder
	counts := make(map[string]int)

	for _, issue := range issues {
		counts[issue.Severity]++
		sb.WriteString(fmt.Sprintf("  [%s] %s:%d - %s (%s)\n",
			issue.Severity, issue.File, issue.Line, issue.Message, issue.Category))
	}

	header := fmt.Sprintf("Security scan: %d critical, %d high, %d medium, %d low\n",
		counts[SeverityCritical], counts[SeverityHigh],
		counts[SeverityMedium], counts[SeverityLow])
	return header + sb.String()
}

// PatternCount returns the total number of registered threat patterns.
func PatternCount() int {
	return len(dangerousSkillPatterns)
}
