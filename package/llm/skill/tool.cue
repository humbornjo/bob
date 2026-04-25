package llmskill

#ToolSkillsList: {
	// Optional category filter (e.g., "mlops")
	category?: string
	// Optional task identifier used to probe the active backend
	task_id?: string @go(TaskID)
}

#ToolSkillView: {
	// Name or path of the skill (e.g., "axolotl" or "03-fine-tuning/axolotl").
	// Qualified names like "plugin:skill" resolve to plugin-provided skills.
	name!: string
	// Optional path to a specific file within the skill (e.g., "references/api.md")
	file_path?: string @go(FilePath)
	// Optional task identifier used to probe the active backend
	task_id?: string @go(TaskID)
	// Apply configured SKILL.md template and inline shell rendering to
	// main skill content. Internal slash/preload callers disable this
	// because they render the skill message themselves.
	preprocess: bool
}
