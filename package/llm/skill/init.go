package llmskill

import (
	_ "embed"
	"github.com/humbornjo/bob/config"
)

var (
	//go:embed tool.cue
	_RAW_TOOL_SCHEMAS string
)

func init() {
	schema, err := config.NewSchema(_RAW_TOOL_SCHEMAS)
	if err != nil {
		panic(err)
	}

	_TOOL_SKILL_VIEW = config.SchemaMustExtractOpenAPI(schema, ToolSkillView{})
	_TOOL_SKILLS_LIST = config.SchemaMustExtractOpenAPI(schema, ToolSkillsList{})
}
