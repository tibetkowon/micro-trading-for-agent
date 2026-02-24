# Skill: Plan Feature Implementation

## Description
This skill ensures that complex features or modifications are carefully planned before any code is written. It prevents the agent from making unauthorized architectural changes, wasting tokens, or causing errors.

## Trigger
Execute this skill when the user requests a new feature, a significant architectural change, or when explicitly asked to "plan a feature".

## Instructions
1. **Analyze Requirements:** Understand the user's request. Read relevant documentation in `docs/` if necessary.
2. **Create Plan Document:** Create a new markdown file in the `docs/plans/` directory (e.g., `docs/plans/buy_order_logic.md`).
3. **Structure the Plan:** The document MUST include:
   - **Goal:** A brief summary of what will be built.
   - **Requirements:** Key functional and non-functional requirements.
   - **Affected Files:** A list of existing files to modify and new files to create.
   - **Implementation Phases:** Break down the work into logical, atomic steps (e.g., Phase 1: DB Schema update, Phase 2: KIS API integration, Phase 3: React UI update).
   - **Verification:** How the feature will be tested or verified.
4. **Wait for Approval:** Do NOT start coding. Output a brief summary of the plan in the chat and explicitly ask the user: "Please review the plan in `docs/plans/[filename].md`. Shall I proceed with Phase 1?"
