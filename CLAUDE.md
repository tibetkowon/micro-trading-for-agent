# Project Overview
This project is an automated AI trading system designed to run efficiently on a low-specification server (NCP Micro, 1GB RAM). 
The system is divided into two main roles:
- **AI Agent Role:**
  - Fetch stock information required for selection.
  - Check account balance, available trading amounts, and applied fees.
  - Execute trade orders.
  - Retrieve order history and execution status for future trading decisions.
- **User Role:**
  - Manage real account settings and credentials via a configuration UI.
  - Monitor trading information including order history, balance, and profit rate.
  - Monitor transaction logs specifically to track KIS (Korea Investment & Securities) API integration errors.

# Tech Stack
- **Backend:** Go (Golang)
- **Frontend:** React
- **Database:** SQLite (Chosen for low memory footprint)

# Initial Goals & Architecture
- Establish a clear separation between Backend and Frontend.
- Set up a CI/CD pipeline using GitHub Actions.
- Design a secure configuration management system for account details and API keys.
- **KIS API Integration Design:**
  - Implement an automatic Access Token refresh mechanism strictly set to a 20-hour interval (to safely preempt the 24-hour expiration).
  - Implement robust API Rate Limiting (TPS control) to comply with KIS API restrictions and prevent IP bans.

# Coding Conventions & Security
- **Commit Messages:** Always append `[skip actions]` to the commit message for non-functional changes (e.g., documentation updates) to prevent unnecessary CI runs.
- **Security:** Never hardcode sensitive data (API keys, account numbers, secrets). All configurations must be managed exclusively through `.env` files.
- **Logging:** Implement structured logging (e.g., JSON format) for backend errors and transactions. All KIS API error logs MUST include: Error Code, Timestamp, and the raw KIS API Response Message.

# Skill Strategy & Context Optimization
To optimize token usage and maintain focus, do not scan the entire workspace indiscriminately. We strictly separate static documentation (`docs/`) from behavioral skill instructions (`.claude/skills/`).

**Available Skills (Behavioral Guidelines & Workflows):**
Always trigger the following skills under their specific conditions by reading and following the corresponding `.md` file in `.claude/skills/`:

1. **Feature Planning (`.claude/skills/plan_feature.md`):** - **Trigger:** Before starting any new feature or architectural change. 
   - **Action:** Create a step-by-step plan in `docs/plans/` and wait for user approval.
2. **Code Verification (`.claude/skills/verify_implementation.md`):** - **Trigger:** After writing/modifying code and BEFORE committing. 
   - **Action:** Run Go build/tests and React lint/build to ensure no broken code is deployed.
3. **Changelog Recording (`.claude/skills/record_changelog.md`):** - **Trigger:** After successfully completing a task or bug fix. 
   - **Action:** Append a brief summary of changes to `docs/changelog.md`.
4. **Code Tutor (`.claude/skills/write_code_tutor.md`):** - **Trigger:** After significant coding or when requested. 
   - **Action:** Generate a Korean explanation of Go/React logic in `docs/reviews/` for the user's learning.
5. **Log Analysis (`.claude/skills/analyze_trade_logs.md`):** - **Trigger:** When investigating KIS API errors or trade failures. 
   - **Action:** Smartly extract and analyze logs using terminal commands without reading entire files.
6. **DB Schema Update (`.claude/skills/update_db_schema.md`):** - **Trigger:** When creating or modifying SQLite tables/models. 
   - **Action:** Update the schema documentation in `docs/db_schema.md`.
7. **Architecture Update (`.claude/skills/update_architecture.md`):** - **Trigger:** When introducing new root folders or major packages. 
   - **Action:** Update the project structure map in `docs/architecture.md`.
8. **README Update (`.claude/skills/update_readme.md`):** - **Trigger:** After major milestones or initial setup. 
   - **Action:** Update the root `README.md` to reflect the current project state.
9. **Context Evolution (`.claude/skills/manage_skills.md`):** - **Trigger:** When discovering new API quirks, project rules, or recurring patterns.
   - **Action:** Document them as new skills or context files so they are not forgotten.
10. **KIS API 구현 (`.claude/skills/implement_kis_feature.md`):** - **Trigger:** KIS API 신규 기능 구현 또는 기존 기능 개선 시.
   - **Action:** `docs/kis-api/` 에서 관련 명세 문서(기본시세/순위분석/종목정보/주문계좌/인증/실시간)를 읽고 올바른 스펙으로 구현.
11. **SKILL.md 생성/갱신 (`.claude/skills/generate_openclaw_spec.md`):** - **Trigger:** 프로젝트 진입점(CLI/API) 변경 시 또는 사용자가 "SKILL.md 업데이트" 요청 시.
   - **Action:** YAML Frontmatter를 포함한 `SKILL.md` 파일을 루트에 생성·업데이트하여 OpenClaw 인덱싱 정보를 유지.