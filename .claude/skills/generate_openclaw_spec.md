---
name: generate-openclaw-spec
description: Generates or updates the root SKILL.md manifest with YAML Frontmatter for OpenClaw indexing.
---
# Skill: Generate OpenClaw SKILL.md (Universal)

## Description
This skill generates or updates the root `SKILL.md` file, the mandatory manifest for OpenClaw. It analyzes the current project's entry point, CLI arguments, and environment requirements to ensure OpenClaw correctly indexes the skill with proper YAML Frontmatter.

## Trigger
- When the project's interface (CLI, API, or Entry Point) is modified.
- When the user explicitly asks to "register this project as an OpenClaw skill" or "update SKILL.md".

## Instructions
1. **Analyze Project Identity:** - Determine the project name from `go.mod`, `package.json`, or the root directory name.
   - Infer the project's core purpose from existing documentation (`README.md`, `CLAUDE.md`).
2. **Analyze Execution Interface:** - Inspect the main entry file (e.g., `main.go`, `index.js`, `app.py`) to identify available commands, flags, or environment variables required for execution.
3. **Generate YAML Frontmatter (Mandatory):** - `name`: A unique kebab-case identifier based on the project name.
   - `description`: A concise, high-level summary of what the tool does (crucial for agent discovery).
   - `metadata`: Include any required environment variables or binary paths discovered during analysis.
4. **Draft Usage Specs:** - Below the Frontmatter, list **Capabilities** (what the tool can do).
   - Provide **Command-line usage** templates based on the actual code structure.
   - Create **Example Prompts** that map natural language intents (primarily in Korean) to specific technical commands.
5. **Format SKILL.md:** - Ensure the file starts with the `---` YAML block followed by the Markdown documentation.
6. **Target File:** Update or create `SKILL.md` in the root directory.
7. **Report (Korean):** Notify the user: "YAML Frontmatter를 포함한 범용 `SKILL.md` 파일이 성공적으로 생성/업데이트되었습니다."