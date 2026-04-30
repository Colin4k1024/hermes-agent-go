package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/hermes-agent/hermes-agent-go/internal/auth"
	"github.com/hermes-agent/hermes-agent-go/internal/middleware"
	"github.com/hermes-agent/hermes-agent-go/internal/objstore"
	"github.com/hermes-agent/hermes-agent-go/internal/skills"
)

type SkillHandler struct {
	minio *objstore.MinIOClient
}

func NewSkillHandler(mc *objstore.MinIOClient) *SkillHandler {
	return &SkillHandler{minio: mc}
}

func (h *SkillHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.minio == nil {
		http.Error(w, "skills storage not configured", http.StatusServiceUnavailable)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/v1/skills")
	path = strings.TrimPrefix(path, "/")

	switch {
	case r.Method == http.MethodGet && path == "":
		h.list(w, r)
	case r.Method == http.MethodPut && path != "":
		h.put(w, r, path)
	case r.Method == http.MethodDelete && path != "":
		h.delete(w, r, path)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type skillListItem struct {
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	Version      string `json:"version,omitempty"`
	Source       string `json:"source,omitempty"`
	UserModified bool   `json:"user_modified"`
}

func (h *SkillHandler) list(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromContext(r.Context())
	if tenantID == "" {
		http.Error(w, "tenant context required", http.StatusUnauthorized)
		return
	}

	loader := skills.NewMinIOSkillLoader(h.minio, tenantID)
	entries, err := loader.LoadAll(r.Context())
	if err != nil {
		slog.Error("list skills failed", "tenant", tenantID, "error", err)
		http.Error(w, "failed to list skills", http.StatusInternalServerError)
		return
	}

	manifest, _ := skills.LoadTenantManifestPublic(r.Context(), h.minio, tenantID)

	items := make([]skillListItem, 0, len(entries))
	for _, e := range entries {
		item := skillListItem{
			Name:        e.Meta.Name,
			Description: e.Meta.Description,
			Version:     e.Meta.Version,
		}
		if manifest != nil {
			if me, ok := manifest.Skills[e.DirName]; ok {
				item.Source = me.Source
				item.UserModified = me.UserModified
			}
		}
		items = append(items, item)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"tenant_id": tenantID,
		"skills":    items,
		"total":     len(items),
	})
}

func (h *SkillHandler) put(w http.ResponseWriter, r *http.Request, name string) {
	ac, ok := auth.FromContext(r.Context())
	if !ok || ac == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	tenantID := ac.TenantID

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB limit
	if err != nil {
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}
	if len(body) == 0 {
		http.Error(w, "body is required (SKILL.md content)", http.StatusBadRequest)
		return
	}

	key := fmt.Sprintf("%s/%s/SKILL.md", tenantID, name)
	if err := h.minio.PutObject(r.Context(), key, body); err != nil {
		slog.Error("put skill failed", "tenant", tenantID, "skill", name, "error", err)
		http.Error(w, "upload failed", http.StatusInternalServerError)
		return
	}

	if err := skills.MarkSkillUserModified(r.Context(), h.minio, tenantID, name); err != nil {
		slog.Warn("mark user_modified failed", "tenant", tenantID, "skill", name, "error", err)
	}

	slog.Info("skill uploaded", "tenant", tenantID, "skill", name)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "uploaded",
		"skill":  name,
	})
}

func (h *SkillHandler) delete(w http.ResponseWriter, r *http.Request, name string) {
	ac, ok := auth.FromContext(r.Context())
	if !ok || ac == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	tenantID := ac.TenantID

	key := fmt.Sprintf("%s/%s/SKILL.md", tenantID, name)
	exists, err := h.minio.ObjectExists(r.Context(), key)
	if err != nil {
		http.Error(w, "check failed", http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, "skill not found", http.StatusNotFound)
		return
	}

	if err := h.minio.DeleteObject(r.Context(), key); err != nil {
		slog.Error("delete skill failed", "tenant", tenantID, "skill", name, "error", err)
		http.Error(w, "delete failed", http.StatusInternalServerError)
		return
	}

	slog.Info("skill deleted", "tenant", tenantID, "skill", name)
	w.WriteHeader(http.StatusNoContent)
}
