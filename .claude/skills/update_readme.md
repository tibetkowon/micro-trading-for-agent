# Skill: Update Project README

## Description
This skill ensures the root `README.md` file accurately reflects the current state of the project. A well-maintained README is essential for understanding the project's purpose, tech stack, and setup instructions at a glance.

## Trigger
Execute this skill after completing the initial project setup, reaching a major milestone (e.g., finishing Phase 1 of a feature), changing the tech stack, or when the user explicitly requests a README update.

## Instructions
1. **Gather Context:** Briefly review `CLAUDE.md`, `docs/architecture.md`, and the latest entries in `docs/changelog.md` to understand the project's current status and rules.
2. **Structure the README:** Update or create the `README.md` file in the root directory. It MUST include:
   - **Project Title & Description:** A clear explanation of the AI trading system.
   - **Tech Stack:** (Go, React, SQLite, etc.)
   - **Prerequisites & Setup:** How to configure the `.env` file and install dependencies.
   - **Run Instructions:** Commands to start the Go backend and React frontend locally.
   - **Project Structure:** A brief overview of the directory layout.
3. **Language:** Write the README in Korean, as requested by the user.
4. **Target File:** Update `README.md` in the root directory.
5. **Output Constraint:** Do not output the entire README content in the chat. Quietly save the file and notify the user: "프로젝트의 최신 상태가 반영된 `README.md` 파일이 업데이트되었습니다."
