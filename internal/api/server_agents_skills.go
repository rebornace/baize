package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/rebornace/baize/internal/skill"
	"github.com/rebornace/baize/internal/store"
)

func (s *Server) handlePutAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing agent id")
		return
	}
	var body struct {
		System      string   `json:"system"`
		Skills      []string `json:"skills"`
		ToolBinding string   `json:"tool_binding"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid json body")
		return
	}
	binding, err := normalizeToolBinding(body.ToolBinding)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	ag := store.Agent{ID: id, System: body.System, Skills: body.Skills, ToolBinding: binding}
	s.Store.UpsertAgent(ag)
	writeJSON(w, http.StatusOK, ag)
}

func normalizeToolBinding(v string) (string, error) {
	v = strings.TrimSpace(strings.ToLower(v))
	switch v {
	case "", store.ToolBindingFloor:
		return "", nil // omit floor in JSON; empty means floor
	case store.ToolBindingExclusive:
		return store.ToolBindingExclusive, nil
	default:
		return "", errors.New("tool_binding must be empty, floor, or exclusive")
	}
}

func (s *Server) handleGetAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing agent id")
		return
	}
	ag, err := s.Store.GetAgent(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "agent_not_found", "unknown agent")
		return
	}
	writeJSON(w, http.StatusOK, ag)
}

type skillSummary struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	DescriptionEN string   `json:"description_en,omitempty"`
	Tools         []string `json:"tools"`
	Source        string   `json:"source"`
}

func skillSummaryFrom(p skill.Package) skillSummary {
	tools := p.Tools
	if tools == nil {
		tools = []string{}
	}
	return skillSummary{
		ID:            p.ID,
		Name:          p.Name,
		Description:   p.Description,
		DescriptionEN: p.DescriptionEN,
		Tools:         tools,
		Source:        p.Source,
	}
}

// skillSummaryForLocale returns a summary whose Description field is already
// localized for the request locale (en / zh-CN). DescriptionEN is still
// included when present so clients can switch without another round-trip.
func skillSummaryForLocale(p skill.Package, locale string) skillSummary {
	sum := skillSummaryFrom(p)
	sum.Description = p.LocalizedDescription(locale)
	return sum
}

func (s *Server) handleListSkills(w http.ResponseWriter, r *http.Request) {
	if s.SkillCatalog == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "skill catalog not configured")
		return
	}
	locale := strings.TrimSpace(r.URL.Query().Get("locale"))
	pkgs := s.SkillCatalog.List()
	out := make([]skillSummary, 0, len(pkgs))
	for _, p := range pkgs {
		out = append(out, skillSummaryForLocale(p, locale))
	}
	writeJSON(w, http.StatusOK, map[string]any{"skills": out})
}

func (s *Server) handleGetSkill(w http.ResponseWriter, r *http.Request) {
	if s.SkillCatalog == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "skill catalog not configured")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing skill id")
		return
	}
	p, ok := s.SkillCatalog.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "skill not found")
		return
	}
	locale := strings.TrimSpace(r.URL.Query().Get("locale"))
	sum := skillSummaryForLocale(p, locale)
	writeJSON(w, http.StatusOK, map[string]any{
		"id":             sum.ID,
		"name":           sum.Name,
		"description":    sum.Description,
		"description_en": sum.DescriptionEN,
		"tools":          sum.Tools,
		"source":         sum.Source,
		"body":           p.LocalizedBody(locale),
	})
}

func (s *Server) handlePostSkill(w http.ResponseWriter, r *http.Request) {
	if s.SkillCatalog == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "skill catalog not configured")
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid multipart form")
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "file is required")
		return
	}
	defer f.Close()
	raw, err := io.ReadAll(f)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	ext := strings.ToLower(filepath.Ext(hdr.Filename))
	var pkg skill.Package
	switch ext {
	case ".md":
		pkg, err = s.SkillCatalog.InstallMD(hdr.Filename, raw)
	case ".zip":
		pkg, err = s.SkillCatalog.InstallZip(raw)
	default:
		writeError(w, http.StatusBadRequest, "invalid_request", "file must be .md or .zip")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, skillSummaryFrom(pkg))
}

func (s *Server) handleDeleteSkill(w http.ResponseWriter, r *http.Request) {
	if s.SkillCatalog == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "skill catalog not configured")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing skill id")
		return
	}
	if err := s.SkillCatalog.DeleteUser(id); err != nil {
		switch {
		case errors.Is(err, skill.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", err.Error())
		case errors.Is(err, skill.ErrBuiltin):
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		default:
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		}
		return
	}
	for _, a := range s.Store.ListAgents() {
		next := make([]string, 0, len(a.Skills))
		changed := false
		for _, sid := range a.Skills {
			if sid == id {
				changed = true
				continue
			}
			next = append(next, sid)
		}
		if changed {
			a.Skills = next
			s.Store.UpsertAgent(a)
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
