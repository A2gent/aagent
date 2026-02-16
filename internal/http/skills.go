package http

import (
	"bufio"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var integrationToolsByProvider = map[string][]string{
	"google_calendar": {"google_calendar_query"},
	"brave_search":    {"brave_search_query"},
	"elevenlabs":      {"elevenlabs_tts"},
	"telegram":        {"telegram_send_message"},
	"exa":             {"exa_search"},
}

var integrationToolNameSet = func() map[string]struct{} {
	out := make(map[string]struct{}, 8)
	for _, names := range integrationToolsByProvider {
		for _, name := range names {
			out[name] = struct{}{}
		}
	}
	return out
}()

type SkillFile struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	RelativePath string `json:"relative_path"`
}

type SkillDiscoverResponse struct {
	Folder string      `json:"folder"`
	Skills []SkillFile `json:"skills"`
}

type SkillBrowseResponse struct {
	Path    string          `json:"path"`
	Entries []MindTreeEntry `json:"entries"`
}

type BuiltInSkill struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Description string `json:"description"`
}

type BuiltInSkillResponse struct {
	Skills []BuiltInSkill `json:"skills"`
}

type IntegrationToolInfo struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema,omitempty"`
}

type IntegrationBackedSkill struct {
	ID       string                `json:"id"`
	Name     string                `json:"name"`
	Provider string                `json:"provider"`
	Mode     string                `json:"mode"`
	Enabled  bool                  `json:"enabled"`
	Tools    []IntegrationToolInfo `json:"tools"`
}

type IntegrationBackedSkillsResponse struct {
	Skills []IntegrationBackedSkill `json:"skills"`
}

func (s *Server) handleListBuiltInSkills(w http.ResponseWriter, r *http.Request) {
	skills := make([]BuiltInSkill, 0, 16)

	toolDefinitions := s.toolManager.GetDefinitions()
	sort.Slice(toolDefinitions, func(i, j int) bool {
		return strings.ToLower(toolDefinitions[i].Name) < strings.ToLower(toolDefinitions[j].Name)
	})
	for _, definition := range toolDefinitions {
		if _, isIntegrationTool := integrationToolNameSet[definition.Name]; isIntegrationTool {
			continue
		}
		skills = append(skills, BuiltInSkill{
			ID:          "tool:" + definition.Name,
			Name:        definition.Name,
			Kind:        "tool",
			Description: strings.TrimSpace(definition.Description),
		})
	}

	s.jsonResponse(w, http.StatusOK, BuiltInSkillResponse{Skills: skills})
}

func (s *Server) handleListIntegrationBackedSkills(w http.ResponseWriter, r *http.Request) {
	integrations, err := s.store.ListIntegrations()
	if err != nil {
		s.errorResponse(w, http.StatusInternalServerError, "Failed to list integrations: "+err.Error())
		return
	}

	defByName := map[string]IntegrationToolInfo{}
	for _, definition := range s.toolManager.GetDefinitions() {
		if _, isIntegrationTool := integrationToolNameSet[definition.Name]; !isIntegrationTool {
			continue
		}
		defByName[definition.Name] = IntegrationToolInfo{
			Name:        definition.Name,
			Description: strings.TrimSpace(definition.Description),
			InputSchema: definition.InputSchema,
		}
	}

	skills := make([]IntegrationBackedSkill, 0, len(integrations))
	for _, integration := range integrations {
		if integration == nil {
			continue
		}
		toolNames := integrationToolsByProvider[strings.TrimSpace(integration.Provider)]
		tools := make([]IntegrationToolInfo, 0, len(toolNames))
		for _, toolName := range toolNames {
			if def, ok := defByName[toolName]; ok {
				tools = append(tools, def)
			}
		}
		sort.Slice(tools, func(i, j int) bool {
			return strings.ToLower(tools[i].Name) < strings.ToLower(tools[j].Name)
		})

		skills = append(skills, IntegrationBackedSkill{
			ID:       integration.ID,
			Name:     integration.Name,
			Provider: integration.Provider,
			Mode:     integration.Mode,
			Enabled:  integration.Enabled,
			Tools:    tools,
		})
	}

	sort.Slice(skills, func(i, j int) bool {
		left := strings.ToLower(skills[i].Provider + "|" + skills[i].Name)
		right := strings.ToLower(skills[j].Provider + "|" + skills[j].Name)
		return left < right
	})

	s.jsonResponse(w, http.StatusOK, IntegrationBackedSkillsResponse{Skills: skills})
}

func (s *Server) handleBrowseSkillDirectories(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSpace(r.URL.Query().Get("path"))
	if path == "" {
		if homeDir, err := os.UserHomeDir(); err == nil {
			path = homeDir
		} else {
			path = string(os.PathSeparator)
		}
	}

	resolvedPath, err := filepath.Abs(path)
	if err != nil {
		s.errorResponse(w, http.StatusBadRequest, "Invalid path")
		return
	}

	entries, err := os.ReadDir(resolvedPath)
	if err != nil {
		s.errorResponse(w, http.StatusBadRequest, "Failed to list directory: "+err.Error())
		return
	}

	respEntries := make([]MindTreeEntry, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		fullPath := filepath.Join(resolvedPath, entry.Name())
		hasChild := directoryHasChildren(fullPath)
		respEntries = append(respEntries, MindTreeEntry{
			Name:     entry.Name(),
			Path:     fullPath,
			Type:     "directory",
			HasChild: hasChild,
		})
	}

	sort.Slice(respEntries, func(i, j int) bool {
		return strings.ToLower(respEntries[i].Name) < strings.ToLower(respEntries[j].Name)
	})

	s.jsonResponse(w, http.StatusOK, SkillBrowseResponse{
		Path:    resolvedPath,
		Entries: respEntries,
	})
}

func (s *Server) handleDiscoverSkills(w http.ResponseWriter, r *http.Request) {
	folder := strings.TrimSpace(r.URL.Query().Get("folder"))
	if folder == "" {
		s.errorResponse(w, http.StatusBadRequest, "folder is required")
		return
	}

	resolvedFolder, err := filepath.Abs(folder)
	if err != nil {
		s.errorResponse(w, http.StatusBadRequest, "Invalid folder path")
		return
	}

	info, err := os.Stat(resolvedFolder)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s.errorResponse(w, http.StatusBadRequest, "Folder does not exist")
			return
		}
		s.errorResponse(w, http.StatusBadRequest, "Failed to access folder: "+err.Error())
		return
	}
	if !info.IsDir() {
		s.errorResponse(w, http.StatusBadRequest, "Path is not a folder")
		return
	}

	skills := make([]SkillFile, 0)
	walkErr := filepath.WalkDir(resolvedFolder, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if d.IsDir() {
			if path != resolvedFolder && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}

		if !isMarkdownFile(d.Name()) {
			return nil
		}

		relPath, err := filepath.Rel(resolvedFolder, path)
		if err != nil {
			return nil
		}

		skills = append(skills, SkillFile{
			Name:         deriveSkillName(path, d.Name()),
			Path:         path,
			RelativePath: filepath.ToSlash(relPath),
		})
		return nil
	})
	if walkErr != nil {
		s.errorResponse(w, http.StatusBadRequest, "Failed to scan skills folder: "+walkErr.Error())
		return
	}

	sort.Slice(skills, func(i, j int) bool {
		return strings.ToLower(skills[i].RelativePath) < strings.ToLower(skills[j].RelativePath)
	})

	s.jsonResponse(w, http.StatusOK, SkillDiscoverResponse{
		Folder: resolvedFolder,
		Skills: skills,
	})
}

func deriveSkillName(path, defaultName string) string {
	name := strings.TrimSpace(strings.TrimSuffix(defaultName, filepath.Ext(defaultName)))

	file, err := os.Open(path)
	if err != nil {
		if name == "" {
			return "Skill"
		}
		return name
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			heading := strings.TrimSpace(strings.TrimLeft(line, "#"))
			if heading != "" {
				return heading
			}
		}
		break
	}

	if name == "" {
		return "Skill"
	}
	return name
}
