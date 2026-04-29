package skills

import (
	"context"
	"fmt"
	"log/slog"
	"path"
	"sort"
	"strings"

	"github.com/hermes-agent/hermes-agent-go/internal/objstore"
)

// MinIOSkillLoader loads skills from S3-compatible object storage, scoped by tenant.
type MinIOSkillLoader struct {
	client   *objstore.MinIOClient
	tenantID string
}

func NewMinIOSkillLoader(client *objstore.MinIOClient, tenantID string) *MinIOSkillLoader {
	return &MinIOSkillLoader{client: client, tenantID: tenantID}
}

func (m *MinIOSkillLoader) LoadAll(ctx context.Context) ([]*SkillEntry, error) {
	prefix := m.tenantID
	keys, err := m.client.ListObjects(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("list skills for tenant %s: %w", m.tenantID, err)
	}

	var entries []*SkillEntry
	for _, key := range keys {
		if !strings.HasSuffix(key, "/SKILL.md") {
			continue
		}

		data, err := m.client.GetObject(ctx, key)
		if err != nil {
			slog.Debug("Failed to read skill from MinIO", "key", key, "error", err)
			continue
		}

		meta, body := ParseFrontmatter(string(data))
		meta.Path = fmt.Sprintf("s3://%s/%s", m.client.Bucket(), key)

		// Extract skill directory name from key: "tenant-X/skill-name/SKILL.md" → "skill-name"
		dirName := path.Base(path.Dir(key))
		if meta.Name == "" {
			meta.Name = dirName
		}

		if !SkillMatchesPlatform(meta) {
			slog.Debug("Skill skipped by platform filter", "key", key, "meta.Platforms", meta.Platforms, "currentPlatform", compilePlatform())
			continue
		}

		entries = append(entries, &SkillEntry{
			Meta:    meta,
			Body:    body,
			DirName: dirName,
		})
	}

	slog.Debug("MinIOSkillLoader keys returned", "tenant", m.tenantID, "count", len(keys), "keys", keys)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Meta.Name < entries[j].Meta.Name
	})

	slog.Debug("Loaded skills from MinIO", "tenant", m.tenantID, "count", len(entries))
	return entries, nil
}

func (m *MinIOSkillLoader) Find(ctx context.Context, name string) (*SkillEntry, error) {
	// Try direct path first
	key := fmt.Sprintf("%s/%s/SKILL.md", m.tenantID, name)
	data, err := m.client.GetObject(ctx, key)
	if err == nil {
		meta, body := ParseFrontmatter(string(data))
		meta.Path = fmt.Sprintf("s3://%s/%s", m.client.Bucket(), key)
		if meta.Name == "" {
			meta.Name = name
		}
		return &SkillEntry{Meta: meta, Body: body, DirName: name}, nil
	}

	// Fallback: load all and search by name
	all, err := m.LoadAll(ctx)
	if err != nil {
		return nil, err
	}
	idx := BuildSkillsIndex(all)
	entry, ok := idx[name]
	if !ok {
		return nil, fmt.Errorf("skill not found: %s (tenant: %s)", name, m.tenantID)
	}
	return entry, nil
}
