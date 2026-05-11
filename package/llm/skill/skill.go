// Package llmskill is a extension pagekage for llm which focus on the
// management of skills.
package llmskill

import (
	_ "embed"
	"regexp"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/encoding/openapi"
	"gopkg.in/yaml.v3"
)

var (
	//go:embed tool.cue
	_TOOL_SCHEMAS_RAW []byte
	_TOOL_SCHEMAS     cue.Value
)

var SKILL_DIR = []string{"~/.hermes/skills", "~/.bob/skills"}

func init() {
	cuetex := cuecontext.New()
	_TOOL_SCHEMAS = cuetex.CompileBytes(_TOOL_SCHEMAS_RAW)
	f, err := openapi.Generate(_TOOL_SCHEMAS, &openapi.Config{})
	if err != nil {
		panic(err)
	}
	topValue := cuetex.BuildFile(f)
	if err := topValue.Err(); err != nil {
		panic(err)
	}

	var schemas map[string]map[string]any
	{
		var document struct {
			Components struct {
				Schemas map[string]map[string]any `json:"schemas"`
			} `json:"components"`
		}

		if err := topValue.Decode(&document); err != nil {
			panic(err)
		}

		schemas = document.Components.Schemas
	}

	ensure_schema := func(key string) map[string]any {
		params, ok := schemas[key]
		if !ok {
			panic("missing schema: " + key)
		}
		if _, ok := params["properties"]; !ok {
			params["properties"] = map[string]any{}
		}

		return params
	}
	_TOOL_SKILL_VIEW = ensure_schema((&ToolSkillView{}).String())
	_TOOL_SKILLS_LIST = ensure_schema((&ToolSkillsList{}).String())
}

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
