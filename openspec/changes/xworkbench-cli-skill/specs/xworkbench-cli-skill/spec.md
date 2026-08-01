# xworkbench-cli-skill

Skill 定义包，将 xworkbench-cli 封装为可被 AI Agent 调用的 Skill。

## ADDED Requirements

### Requirement: Skill package structure
The skill package SHALL be a self-contained directory under `tools/xworkbench-cli-skill/`.

#### Scenario: Package contents
- **WHEN** Agent looks at the skill directory
- **THEN** it contains: SKILL.md, xworkbench-cli binary, README.md

### Requirement: SKILL.md format
SKILL.md SHALL use YAML frontmatter with xw_command pointing to xworkbench-cli binary.

### Requirement: xw_params definition
SKILL.md SHALL define xw_params describing all available subcommands and their parameters.

### Requirement: Agent can invoke skill
The skill SHALL be invokable by Claude Code and similar agents using standard skill invocation.
