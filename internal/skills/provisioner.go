package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/objstore"
	"github.com/hermes-agent/hermes-agent-go/internal/store"
)

// TenantManifest tracks which skills a tenant has and their modification state.
type TenantManifest struct {
	Version  int                        `json:"version"`
	Skills   map[string]TenantSkillMeta `json:"skills"`
	SyncedAt time.Time                  `json:"synced_at"`
}

// TenantSkillMeta holds per-skill metadata stored in the manifest.
type TenantSkillMeta struct {
	Source       string    `json:"source"`
	UserModified bool      `json:"user_modified"`
	InstalledAt  time.Time `json:"installed_at"`
}

const manifestKey = "/.manifest.json"

func loadManifest(ctx context.Context, mc *objstore.MinIOClient, tenantID string) (*TenantManifest, error) {
	data, err := mc.GetObject(ctx, tenantID+manifestKey)
	if err != nil {
		return &TenantManifest{Version: 1, Skills: make(map[string]TenantSkillMeta)}, nil
	}
	var m TenantManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return &TenantManifest{Version: 1, Skills: make(map[string]TenantSkillMeta)}, nil
	}
	if m.Skills == nil {
		m.Skills = make(map[string]TenantSkillMeta)
	}
	return &m, nil
}

func saveManifest(ctx context.Context, mc *objstore.MinIOClient, tenantID string, m *TenantManifest) error {
	m.SyncedAt = time.Now()
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	return mc.PutObject(ctx, tenantID+manifestKey, data)
}

// LoadTenantManifestPublic returns the tenant skill manifest for read-only use.
func LoadTenantManifestPublic(ctx context.Context, mc *objstore.MinIOClient, tenantID string) (*TenantManifest, error) {
	return loadManifest(ctx, mc, tenantID)
}

// MarkSkillUserModified flags a skill as manually modified by the tenant user.
func MarkSkillUserModified(ctx context.Context, mc *objstore.MinIOClient, tenantID, skillName string) error {
	m, err := loadManifest(ctx, mc, tenantID)
	if err != nil {
		return err
	}
	meta := m.Skills[skillName]
	meta.UserModified = true
	if meta.InstalledAt.IsZero() {
		meta.InstalledAt = time.Now()
	}
	m.Skills[skillName] = meta
	return saveManifest(ctx, mc, tenantID, m)
}

// defaultSoulContent mirrors cli.DefaultSoulMD to avoid import cycle (skills → cli → agent → skills).
const defaultSoulContent = `# Hermes Agent

You are Hermes, an AI assistant built by Nous Research.

## Core Identity
- You are helpful, accurate, and proactive.
- You use available tools to accomplish tasks effectively.
- You prioritize user safety and warn before destructive operations.
- You are transparent about your capabilities and limitations.

## Personality
- Friendly but professional tone.
- Concise responses unless detail is requested.
- Offer actionable suggestions when appropriate.
- Admit uncertainty rather than guessing.

## Principles
1. Use tools when they can provide better answers than your training data alone.
2. Ask for clarification when instructions are ambiguous.
3. Break complex tasks into manageable steps.
4. Preserve user data and avoid unintended side effects.
5. Respect privacy — never log or transmit sensitive information unnecessarily.
`

const maxSoulBytes = 64 * 1024

type Provisioner struct {
	minio      *objstore.MinIOClient
	bundledDir string
}

func validateTenantID(id string) error {
	if id == "" || id == "." || id == ".." || strings.ContainsAny(id, "/\\") {
		return fmt.Errorf("invalid tenant ID: %q", id)
	}
	return nil
}

func NewProvisioner(minio *objstore.MinIOClient, bundledDir string) *Provisioner {
	return &Provisioner{minio: minio, bundledDir: bundledDir}
}

func (p *Provisioner) Provision(ctx context.Context, tenantID string) error {
	if err := validateTenantID(tenantID); err != nil {
		return err
	}
	var errs []string
	if err := p.ProvisionSoul(ctx, tenantID); err != nil {
		slog.Error("provision soul failed", "tenant", tenantID, "error", err)
		errs = append(errs, err.Error())
	}
	if err := p.ProvisionSkills(ctx, tenantID); err != nil {
		slog.Error("provision skills failed", "tenant", tenantID, "error", err)
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return fmt.Errorf("provision partial failure: %s", strings.Join(errs, "; "))
	}
	slog.Info("tenant provisioning complete", "tenant", tenantID)
	return nil
}

func (p *Provisioner) ProvisionSoul(ctx context.Context, tenantID string) error {
	key := tenantID + "/SOUL.md"
	exists, err := p.minio.ObjectExists(ctx, key)
	if err != nil {
		return fmt.Errorf("check soul exists: %w", err)
	}
	if exists {
		slog.Debug("soul already exists, skipping", "tenant", tenantID)
		return nil
	}
	if err := p.minio.PutObject(ctx, key, []byte(defaultSoulContent)); err != nil {
		return fmt.Errorf("upload soul: %w", err)
	}
	slog.Info("provisioned tenant soul", "tenant", tenantID, "key", key)
	return nil
}

func (p *Provisioner) ProvisionSkills(ctx context.Context, tenantID string) error {
	if p.bundledDir == "" {
		return nil
	}
	if _, err := os.Stat(p.bundledDir); os.IsNotExist(err) {
		slog.Warn("bundled skills directory not found", "dir", p.bundledDir)
		return nil
	}

	var uploaded, skipped int
	err := filepath.Walk(p.bundledDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() || !strings.HasSuffix(info.Name(), "SKILL.md") {
			return nil
		}

		rel, err := filepath.Rel(p.bundledDir, path)
		if err != nil {
			return nil
		}
		key := tenantID + "/" + filepath.ToSlash(rel)

		exists, err := p.minio.ObjectExists(ctx, key)
		if err != nil {
			slog.Warn("check skill exists failed", "key", key, "error", err)
			return nil
		}
		if exists {
			skipped++
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			slog.Warn("read bundled skill failed", "path", path, "error", err)
			return nil
		}
		if err := p.minio.PutObject(ctx, key, data); err != nil {
			slog.Warn("upload skill failed", "key", key, "error", err)
			return nil
		}
		uploaded++
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk bundled skills: %w", err)
	}

	slog.Info("tenant skill sync complete", "tenant", tenantID, "uploaded", uploaded, "skipped", skipped)
	return nil
}

func (p *Provisioner) SyncAllTenants(ctx context.Context, tenantStore store.TenantStore) error {
	tenants, _, err := tenantStore.List(ctx, store.ListOptions{Limit: 1000})
	if err != nil {
		return fmt.Errorf("list tenants: %w", err)
	}

	slog.Info("starting tenant sync", "count", len(tenants))
	for _, t := range tenants {
		if err := p.Provision(ctx, t.ID); err != nil {
			slog.Error("sync tenant failed", "tenant", t.ID, "error", err)
		}
	}
	slog.Info("tenant sync complete", "count", len(tenants))
	return nil
}
