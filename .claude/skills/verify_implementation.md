---
name: verify-implementation
description: Enforces self-verification of Go and React code quality and build stability before git commits.
---
# Skill: Verify Implementation Before Commit

## Description
This skill enforces self-verification to ensure code quality and prevent broken builds. It must be run before finalizing any code changes.

## Trigger
Execute this skill immediately after writing or modifying Go or React code, and BEFORE creating a git commit or telling the user the task is complete.

## Instructions
1. **Backend (Go) Verification:**
   - Run `go fmt ./...` to ensure proper formatting.
   - Run `go build ./...` to check for compilation errors.
   - Run `go test ./...` if test files exist.
2. **Frontend (React) Verification:**
   - Run `npm run lint` (or `pnpm lint`) to check for syntax and style errors.
   - Run `npm run build` (or `pnpm build`) to ensure the Vite project builds successfully.
3. **Fix Issues:** If ANY step fails, you MUST automatically fix the errors and re-run the verification before proceeding.
4. **Report:** Once all checks pass, quietly notify the user in Korean: "모든 코드 검증(Go 빌드/테스트, React 린트/빌드)이 성공적으로 완료되었습니다."