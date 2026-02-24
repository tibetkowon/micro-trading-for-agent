# Skill: Record Feature Changelog and History

## Description
This skill maintains a chronological history of all feature additions, modifications, and bug fixes. It acts as the primary memory bank for the agent across different sessions, significantly reducing context-loading tokens and allowing the user to simply request to "continue from the last task."

## Trigger
Execute this skill immediately after successfully completing a feature implementation, bug fix, or any significant configuration change.

## Instructions
1. **Summarize Changes:** Briefly summarize what was changed, why it was changed, and how it was implemented.
2. **Format as Markdown List:** Append the new entry to `docs/changelog.md` under the current date header (YYYY-MM-DD).
3. **Required Information:** Each entry MUST include:
   - **Type:** [Feature], [Fix], [Refactor], or [Config]
   - **Description:** A concise explanation of the change.
   - **Files Touched:** A list of the primary files modified or created.
   - **Pending/Next Steps:** Any remaining tasks or known issues to address in the next session.
4. **Target File:** Update `docs/changelog.md`, ensuring the newest entries are at the top (reverse chronological order).
5. **Output Constraint:** Do not output the entire changelog in the chat. Quietly update the file and simply notify the user: "The changelog (`docs/changelog.md`) has been updated successfully."
