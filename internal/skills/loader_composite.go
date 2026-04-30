package skills

import (
	"context"
	"fmt"
	"log/slog"
)

// CompositeSkillLoader merges results from multiple SkillLoaders.
// Primary loader (e.g. MinIO per-tenant) runs first; fallback loader
// (e.g. local bundled skills) fills in any skills not already present.
type CompositeSkillLoader struct {
	loaders []SkillLoader
}

func NewCompositeSkillLoader(loaders ...SkillLoader) *CompositeSkillLoader {
	return &CompositeSkillLoader{loaders: loaders}
}

func (c *CompositeSkillLoader) LoadAll(ctx context.Context) ([]*SkillEntry, error) {
	seen := make(map[string]bool)
	var merged []*SkillEntry

	for _, loader := range c.loaders {
		entries, err := loader.LoadAll(ctx)
		if err != nil {
			slog.Debug("CompositeSkillLoader: loader failed, skipping", "error", err)
			continue
		}
		for _, e := range entries {
			key := e.Meta.Name
			if key == "" {
				key = e.DirName
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			merged = append(merged, e)
		}
	}

	return merged, nil
}

func (c *CompositeSkillLoader) Find(ctx context.Context, name string) (*SkillEntry, error) {
	for _, loader := range c.loaders {
		entry, err := loader.Find(ctx, name)
		if err == nil {
			return entry, nil
		}
	}
	return nil, fmt.Errorf("skill not found in any loader: %s", name)
}
