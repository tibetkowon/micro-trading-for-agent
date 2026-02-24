# Skill: Update Project Architecture Documentation

## Description
This skill ensures that the high-level project architecture and directory structure documentation is always accurate. As this is a Monorepo containing both a Go backend and a React frontend, maintaining a clear map of responsibilities prevents the agent from misplacing files or introducing architectural violations.

## Trigger
Execute this skill whenever you create a new root-level folder, introduce a new major Go package inside `/internal` or `/cmd`, create a new React page routing, or when the user explicitly requests an architecture documentation update.

## Instructions
1. **Scan Directory Structure:** Analyze the current state of both the `/backend` and `/frontend` directories.
2. **Document Responsibilities:** Update `docs/architecture.md` to reflect the latest structure. For each major directory or key file, you MUST explain:
   - Its primary role (e.g., Domain logic, UI Components, API Handlers).
   - What should be placed inside it.
   - What should NOT be placed inside it (to enforce clean architecture).
3. **Format as a Tree:** Use markdown code blocks to visually represent the directory tree before explaining the components.
4. **Target File:** Overwrite or carefully update `docs/architecture.md`.
5. **Output Constraint:** Do not output the entire architecture document in the chat. Quietly save the changes and simply notify the user: "The project architecture documentation (`docs/architecture.md`) has been updated successfully."
