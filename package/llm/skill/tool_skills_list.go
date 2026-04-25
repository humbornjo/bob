package llmskill

import (
	"context"
	"encoding/json"
	"io/fs"
	"iter"
	"os"
	"path/filepath"
	"strings"

	anyllm "github.com/mozilla-ai/any-llm-go"

	llmtool "github.com/humbornjo/bob/package/llm/tool"
)

// Params Implementation ---------------------------------------------
var _ llmtool.Params = (*ToolSkillsList)(nil)

var _TOOL_SKILLS_LIST map[string]any

func (t *ToolSkillsList) String() string {
	return "ToolSkillsList"
}

func (t *ToolSkillsList) Schema() map[string]any {
	return _TOOL_SKILLS_LIST
}

// Toolx Implementation ----------------------------------------------
var _ llmtool.Toolx = (*ToolSkillsList)(nil)

func NewToolSkillsList() llmtool.Toolx {
	return &ToolSkillsList{}
}

func (t *ToolSkillsList) Tool() anyllm.Tool {
	return anyllm.Tool{
		Type:     "function",
		Function: t.Function(),
	}
}

func (t *ToolSkillsList) Function() anyllm.Function {
	return anyllm.Function{
		Name: "skills_list",
		Description: `View the content of a skill or a specific file within a skill directory.

    Returns:
        JSON string with skill content or error message`,
		Parameters: _TOOL_SKILLS_LIST,
	}
}

func (t *ToolSkillsList) Execute(ctx context.Context, args string, opts ...llmtool.Option) (string, error) {
	if err := json.Unmarshal([]byte(args), t); err != nil {
		return "", err
	}

	type Entry struct {
		Name        string `json:"name"`
		Category    string `json:"category"`
		Description string `json:"description"`
	}
	type Output = []Entry
	output := Output{}

	// Walk each directory to find all the SKILL.md
	for dir := range func(dirs ...string) iter.Seq[string] {
		return func(yield func(string) bool) {
			home, _ := os.UserHomeDir()
			for _, dir := range dirs {
				if strings.HasPrefix(dir, "~/") {
					dir = filepath.Join(home, dir[1:])
				}
				dir, _ = filepath.Abs(dir)
				if !yield(dir) {
					return
				}
			}
		}
	}(SKILL_DIR...) {
		skillfs := os.DirFS(dir)
		_ = fs.WalkDir(skillfs, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil || d == nil || d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, "/SKILL.md") {
				return nil
			}

			category := func(path string) string {
				if parts := strings.Split(path, "/"); len(parts) >= 3 {
					return parts[0]
				}
				return ""
			}(path)
			name, description := func(skillfs fs.FS, path string) (string, string) {
				content, err := fs.ReadFile(skillfs, path)
				if err != nil {
					return "", ""
				}
				var fm Frontmatter
				fm.Parse(string(content))
				return fm.Name, fm.Description
			}(skillfs, path)
			output = append(output, Entry{Category: category, Name: name, Description: description})
			return nil
		})
	}

	jsonb, err := json.Marshal(output)
	if err != nil {
		return "", err
	}

	return string(jsonb), nil
}

func (t *ToolSkillsList) ExecuteStream(ctx context.Context, args string, opts ...llmtool.Option,
) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) { yield(t.Execute(ctx, args, opts...)) }
}
