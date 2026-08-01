# xworkbench-cli-skill Implementation Tasks

## 1. Project Setup

- [x] 1.1 Create `cmd/xworkbench-cli/` directory structure
- [x] 1.2 Initialize Go module with go.mod
- [x] 1.3 Create main.go with cobra-free CLI framework (using flag + manual subcommand)

## 2. CLI Core Framework

- [x] 2.1 Implement base command with --server flag and JSON output
- [x] 2.2 Implement help text system
- [x] 2.3 Implement HTTP client helper functions
- [x] 2.4 Implement error handling and JSON response formatting

## 3. Task Subcommands

- [x] 3.1 Implement `task list` - GET /api/tasks
- [x] 3.2 Implement `task create` - POST /api/tasks
- [x] 3.3 Implement `task get` - GET /api/tasks/{id}
- [x] 3.4 Implement `task update` - PUT /api/tasks/{id}
- [x] 3.5 Implement `task run` - POST /api/tasks/{id}/run
- [x] 3.6 Implement `task cancel` - POST /api/tasks/{id}/cancel
- [x] 3.7 Implement `task delete` - DELETE /api/tasks/{id}

## 4. Exec Subcommands

- [x] 4.1 Implement `exec list` - GET /api/executions
- [x] 4.2 Implement `exec get` - GET /api/executions/{id}
- [x] 4.3 Implement `exec evaluate` - POST /api/executions/{id}/evaluate
- [x] 4.4 Implement `exec cancel` - POST /api/executions/{id}/cancel
- [x] 4.5 Implement `exec continue` - POST /api/executions/{id}/continue

## 5. Experience Subcommands

- [x] 5.1 Implement `experience list` - GET /api/experiences
- [x] 5.2 Implement `experience get` - GET /api/experiences/{id}
- [x] 5.3 Implement `experience create` - POST /api/experiences
- [x] 5.4 Implement `experience update` - PUT /api/experiences/{id}
- [x] 5.5 Implement `experience delete` - DELETE /api/experiences/{id}

## 6. Scheduled Subcommands

- [x] 6.1 Implement `scheduled list` - GET /api/scheduled
- [x] 6.2 Implement `scheduled get` - GET /api/scheduled/{id}
- [x] 6.3 Implement `scheduled create` - POST /api/scheduled
- [x] 6.4 Implement `scheduled update` - PUT /api/scheduled/{id}
- [x] 6.5 Implement `scheduled delete` - DELETE /api/scheduled/{id}
- [x] 6.6 Implement `scheduled run` - POST /api/scheduled/{id}/run-now
- [x] 6.7 Implement `scheduled toggle` - POST /api/scheduled/{id}/toggle

## 7. Todo Subcommands

- [x] 7.1 Implement `todo list` - GET /api/todo
- [x] 7.2 Implement `todo add` - POST /api/todo
- [x] 7.3 Implement `todo toggle` - PUT /api/todo/{line_no}
- [x] 7.4 Implement `todo edit` - PUT /api/todo/{line_no}/edit

## 8. Config & Models

- [x] 8.1 Implement `config get` - GET /api/config
- [x] 8.2 Implement `config set` - PUT /api/config
- [x] 8.3 Implement `models list` - GET /api/models

## 9. Other Commands

- [x] 9.1 Implement `stats` - GET /api/stats

## 10. Dir-Shortcut Subcommands

- [x] 10.1 Implement `dir-shortcut list` - GET /api/dir-shortcuts
- [x] 10.2 Implement `dir-shortcut create` - POST /api/dir-shortcuts
- [x] 10.3 Implement `dir-shortcut update` - PUT /api/dir-shortcuts/{id}
- [x] 10.4 Implement `dir-shortcut delete` - DELETE /api/dir-shortcuts/{id}
- [x] 10.5 Implement `dir-shortcut open` - POST /api/dir-shortcuts/{id}/open

## 11. Web-Link Subcommands

- [x] 11.1 Implement `web-link list` - GET /api/web-links
- [x] 11.2 Implement `web-link create` - POST /api/web-links
- [x] 11.3 Implement `web-link update` - PUT /api/web-links/{id}
- [x] 11.4 Implement `web-link delete` - DELETE /api/web-links/{id}
- [x] 11.5 Implement `web-link open` - POST /api/links/open

## 12. Skill Package

- [x] 12.1 Create `tools/xworkbench-cli-skill/SKILL.md` with xw_command and xw_params
- [x] 12.2 Create `tools/xworkbench-cli-skill/README.md` with usage instructions
- [x] 12.3 Build and place xworkbench-cli binary in skill directory

## 13. Build & Release

- [x] 13.1 Add cross-platform build script
- [x] 13.2 Test all subcommands locally
- [x] 13.3 Verify JSON output format
