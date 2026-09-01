package SCRP

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// SkillInfo — информация о скиле для API.
type SkillInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`
}

// handleAPISkills возвращает JSON массив скилов из .pi/skills/.
func handleAPISkills(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "no-store")

	skillsDir := ".pi/skills"
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	var skills []SkillInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillName := e.Name()
		skillPath := filepath.Join(skillsDir, skillName, "SKILL.md")

		data, err := os.ReadFile(skillPath)
		if err != nil {
			continue // пропускаем если нет SKILL.md
		}

		name, description := parseSKILLMd(data)
		skills = append(skills, SkillInfo{
			Name:        name,
			Description: description,
			Path:        skillPath,
		})
	}

	if skills == nil {
		skills = []SkillInfo{}
	}

	json.NewEncoder(w).Encode(skills)
}

// parseSKILLMd читает первый заголовок # как name, остальное как description.
func parseSKILLMd(data []byte) (name, description string) {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	firstLine := true
	var lines []string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if firstLine && strings.HasPrefix(line, "# ") {
			name = strings.TrimSpace(strings.TrimPrefix(line, "# "))
			firstLine = false
			continue
		}
		firstLine = false
		if line != "" {
			lines = append(lines, line)
		}
	}

	description = strings.Join(lines, "\n")
	if name == "" {
		name = "Unknown"
	}
	return
}
