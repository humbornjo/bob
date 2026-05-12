// Package llmskill is a extension pagekage for llm which focus on the
// management of skills.
package llmskill

import (
	_ "embed"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var SKILL_DIR = []string{"~/.hermes/skills", "~/.bob/skills"}

type Frontmatter struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (f *Frontmatter) Parse(content string) {
	if !strings.HasPrefix(content, "---") {
		return
	}

	content = content[3:]
	endMatch := regexp.MustCompile(`\n---\s*\n`).FindStringIndex(content)
	if endMatch == nil {
		return
	}

	yamlContent := content[:endMatch[0]]

	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(yamlContent), &parsed); err != nil {
		return
	}

	if name, ok := parsed["name"].(string); ok {
		f.Name = name
	}
	if desc, ok := parsed["description"].(string); ok {
		f.Description = desc
	}
}
