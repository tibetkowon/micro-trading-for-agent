# Skill: Analyze Trade and Error Logs

## Description
This skill is used to investigate issues, particularly KIS API errors or failed trade transactions, without blindly reading massive log files. It helps pinpoint the root cause efficiently and saves token usage.

## Trigger
Execute this skill when the user reports an error, asks to check the logs, or when an automated trade fails unexpectedly.

## Instructions
1. **Locate Logs:** Identify the relevant log files (e.g., backend application logs, KIS API transaction logs).
2. **Filter and Extract:** Use terminal commands (like `grep`, `tail`, or `awk`) to extract ONLY the relevant timeframes or error codes (e.g., KIS API error codes). Do NOT read the entire log file into memory.
3. **Analyze:** Identify the root cause of the error based on the structured log data (Error Code, Timestamp, KIS API Response Message).
4. **Report (Korean):** Provide a concise summary of the issue to the user in Korean. Include the exact error message, the inferred cause, and a proposed solution. Output format: "로그 분석 결과: [원인 및 해결책]"
