package llmskill

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"iter"
	"os"
	"path/filepath"
	"strings"

	anyllm "github.com/mozilla-ai/any-llm-go"

	llmtool "github.com/humbornjo/bob/package/llm/tool"
)

// Params Implementation ---------------------------------------------
var _ llmtool.Params = (*ToolSkillView)(nil)

var _TOOL_SKILL_VIEW map[string]any

func (t *ToolSkillView) Schema() map[string]any {
	return _TOOL_SKILL_VIEW
}

// Toolx Implementation ----------------------------------------------
var _ llmtool.Toolx = (*ToolSkillView)(nil)

func NewToolSkillView() llmtool.Toolx {
	return &ToolSkillView{}
}

func (t *ToolSkillView) Tool() anyllm.Tool {
	return anyllm.Tool{
		Type:     "function",
		Function: t.Function(),
	}
}

func (t *ToolSkillView) Function() anyllm.Function {
	return anyllm.Function{
		Name: "skill_view",
		Description: `List all available skills (progressive disclosure tier 1 - minimal metadata).

    Returns only name + description to minimize token usage. Use skill_view to
    load full content, tags, related files, etc.

    Returns:
        JSON string with minimal skill info: name, description, category`,
		Parameters: _TOOL_SKILL_VIEW,
	}
}

func (t *ToolSkillView) Execute(ctx context.Context, args string, opts ...llmtool.Option) (string, error) {
	if err := json.Unmarshal([]byte(args), t); err != nil {
		return "", err
	}

	type LinkedFiles struct {
		References []string `json:"references,omitempty"`
		Templates  []string `json:"templates,omitempty"`
		Assets     []string `json:"assets,omitempty"`
		Scripts    []string `json:"scripts,omitempty"`
	}

	type Output struct {
		Success     bool         `json:"success"`
		Name        string       `json:"name"`
		Description string       `json:"description"`
		Content     string       `json:"content"`
		Path        string       `json:"path"`
		SkillDir    string       `json:"skill_dir,omitempty"`
		LinkedFiles *LinkedFiles `json:"linked_files,omitempty"`
		File        string       `json:"file,omitempty"`
		FileType    string       `json:"file_type,omitempty"`
		IsBinary    bool         `json:"is_binary,omitempty"`
	}

	// Search for skill in all skill directories
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

		// Try direct path first (e.g., "category/skill")
		directPath := t.Name
		directSkillDir := filepath.Join(dir, directPath)
		directSkillMd := filepath.Join(directSkillDir, "SKILL.md")

		var skillMdPath string
		var skillDir string

		if info, err := os.Stat(directSkillMd); err == nil && !info.IsDir() {
			skillMdPath = directSkillMd
			skillDir = directSkillDir
		} else if info, err := os.Stat(directPath + ".md"); err == nil && !info.IsDir() {
			skillMdPath = directPath + ".md"
		}

		// Search by walking the directory tree
		if skillMdPath == "" {
			_ = fs.WalkDir(skillfs, ".", func(path string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil
				}
				if d.Name() == "SKILL.md" {
					if parentName := filepath.Base(filepath.Dir(path)); parentName == t.Name {
						skillMdPath = filepath.Join(dir, path)
						skillDir = filepath.Join(dir, filepath.Dir(path))
						return fs.SkipAll
					}
				}
				return nil
			})
		}

		if skillMdPath == "" {
			continue
		}

		// Read the SKILL.md file
		content, err := os.ReadFile(skillMdPath)
		if err != nil {
			continue
		}

		var fm Frontmatter
		fm.Parse(string(content))

		skillName := fm.Name
		if skillName == "" {
			skillName = filepath.Base(skillDir)
		}

		// If a specific file path is requested, read that instead
		if t.FilePath != "" && skillDir != "" {
			targetFile := filepath.Join(skillDir, t.FilePath)

			// Security: Verify the resolved path is within skill directory
			absTarget, _ := filepath.Abs(targetFile)
			absSkillDir, _ := filepath.Abs(skillDir)
			if !strings.HasPrefix(absTarget, absSkillDir+string(filepath.Separator)) {
				jsonb, _ := json.Marshal(map[string]any{
					"success": false,
					"error":   "Path traversal detected. File path must be within skill directory.",
				})
				return string(jsonb), nil
			}

			if _, err := os.Stat(targetFile); os.IsNotExist(err) {
				jsonb, _ := json.Marshal(map[string]any{
					"success": false,
					"error":   fmt.Sprintf("File '%s' not found in skill '%s'.", t.FilePath, t.Name),
				})
				return string(jsonb), nil
			}

			fileContent, err := os.ReadFile(targetFile)
			if err != nil {
				jsonb, _ := json.Marshal(map[string]any{
					"success": false,
					"error":   fmt.Sprintf("Failed to read file '%s': %v", t.FilePath, err),
				})
				return string(jsonb), nil
			}

			output := Output{
				Success:  true,
				Name:     skillName,
				Content:  string(fileContent),
				Path:     t.FilePath,
				SkillDir: skillDir,
				File:     t.FilePath,
				FileType: filepath.Ext(targetFile),
			}

			jsonb, err := json.Marshal(output)
			if err != nil {
				return "", err
			}
			return string(jsonb), nil
		}

		// Collect linked files
		linkedFiles := &LinkedFiles{}
		hasLinkedFiles := false

		if skillDir != "" {
			referencesDir := filepath.Join(skillDir, "references")
			if entries, err := os.ReadDir(referencesDir); err == nil {
				for _, entry := range entries {
					if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
						linkedFiles.References = append(linkedFiles.References, "references/"+entry.Name())
						hasLinkedFiles = true
					}
				}
			}

			templatesDir := filepath.Join(skillDir, "templates")
			if entries, err := os.ReadDir(templatesDir); err == nil {
				for _, entry := range entries {
					if !entry.IsDir() {
						ext := filepath.Ext(entry.Name())
						if ext == ".md" || ext == ".py" || ext == ".yaml" || ext == ".yml" || ext == ".json" || ext == ".tex" || ext == ".sh" {
							linkedFiles.Templates = append(linkedFiles.Templates, "templates/"+entry.Name())
							hasLinkedFiles = true
						}
					}
				}
			}

			assetsDir := filepath.Join(skillDir, "assets")
			_ = filepath.WalkDir(assetsDir, func(path string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil
				}
				rel, _ := filepath.Rel(skillDir, path)
				linkedFiles.Assets = append(linkedFiles.Assets, rel)
				hasLinkedFiles = true
				return nil
			})

			scriptsDir := filepath.Join(skillDir, "scripts")
			if entries, err := os.ReadDir(scriptsDir); err == nil {
				for _, entry := range entries {
					if !entry.IsDir() {
						ext := filepath.Ext(entry.Name())
						if ext == ".py" || ext == ".sh" || ext == ".bash" || ext == ".js" || ext == ".ts" || ext == ".rb" {
							linkedFiles.Scripts = append(linkedFiles.Scripts, "scripts/"+entry.Name())
							hasLinkedFiles = true
						}
					}
				}
			}
		}

		// Get relative path
		relPath, _ := filepath.Rel(dir, skillMdPath)
		if relPath == "" {
			relPath = skillMdPath
		}

		output := Output{
			Success:     true,
			Name:        skillName,
			Description: fm.Description,
			Content:     string(content),
			Path:        relPath,
			SkillDir:    skillDir,
		}

		if hasLinkedFiles {
			output.LinkedFiles = linkedFiles
		}

		jsonb, err := json.Marshal(output)
		if err != nil {
			return "", err
		}
		return string(jsonb), nil
	}

	// Skill not found
	jsonb, _ := json.Marshal(map[string]any{
		"success": false,
		"error":   fmt.Sprintf("Skill '%s' not found.", t.Name),
	})
	return string(jsonb), nil
}

func (t *ToolSkillView) ExecuteStream(ctx context.Context, args string, opts ...llmtool.Option,
) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) { yield(t.Execute(ctx, args, opts...)) }
}
