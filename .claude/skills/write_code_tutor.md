# Skill: Write Code Review and Tutor Document

## Description
This skill acts as a personal coding tutor for the user, who is currently learning Go and React. It generates an educational review document explaining the newly written or modified code, helping the user understand the core logic and language-specific concepts.

## Trigger
Execute this skill immediately after completing a significant feature implementation, or when the user explicitly requests a code review or explanation.

## Instructions
1. **Analyze the Code:** Review the Go (Backend) and React (Frontend) code that was just written or modified.
2. **Create Tutor Document:** Create a new markdown file in the `docs/reviews/` directory (e.g., `docs/reviews/kis_api_auth_review.md`).
3. **Structure the Document:** The document MUST be written in Korean for the user's convenience and include:
   - **Overview (개요):** A simple explanation of what the code does.
   - **Go Backend Logic (Go 백엔드 해설):** Explain the core Go logic. Explicitly explain any Go-specific concepts used (e.g., pointers, structs, Fiber routing, goroutines).
   - **React Frontend Logic (React 프론트엔드 해설):** Explain the React components. Explicitly explain React hooks (useState, useEffect) or Tailwind CSS classes used.
   - **Key Takeaways (핵심 요약):** Important design patterns or syntax the user should remember from this update.
4. **Target File:** Save the document in the `docs/reviews/` folder.
5. **Output Constraint:** Do not output the entire review in the chat. Quietly create the file and notify the user: "코드 작성의 원리와 언어적 특징을 설명한 리뷰 문서가 `docs/reviews/[filename].md`에 생성되었습니다. 학습을 위해 확인해 보세요."
