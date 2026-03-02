# Skill: Record Changelog

## Description
모든 기능 구현 또는 버그 수정 완료 후 변경 이력을 `docs/changelog.md`에 기록한다.
변경 이력은 다음 세션에서 컨텍스트를 빠르게 파악하는 데 핵심적으로 활용된다.

## Trigger
- 기능 구현 또는 버그 수정이 성공적으로 완료된 후
- 코드 검증(verify_implementation) 스킬 통과 후
- 사용자가 명시적으로 "changelog 기록해줘" 요청 시

## Instructions

1. **위치 확인:** `docs/changelog.md` 파일이 없으면 새로 생성한다.

2. **최신 항목을 맨 위에 추가 (prepend):**
   - 파일 첫 번째 `##` 헤딩 앞에 새 항목을 삽입한다.
   - 날짜 형식: `## YYYY-MM-DD — 변경 제목`

3. **항목 구조:**
   ```markdown
   ## YYYY-MM-DD — 변경 제목

   - **파일명**: 변경 내용 한 줄 요약
   - **파일명**: 변경 내용 한 줄 요약
   ```
   - 파일 단위로 bullet 작성
   - 신규 파일은 **(신규)** 표시
   - 기술 용어는 영어 유지, 설명은 한국어

4. **간결하게:** 항목 하나당 5줄 이내. 구현 세부사항보다 "무엇이 달라졌는가"에 집중.

5. **Output Constraint:** 채팅에 전체 내용을 출력하지 말 것. 파일 저장 후 다음 메시지만 출력:
   `"변경 이력이 \`docs/changelog.md\`에 기록되었습니다."`

## Example

```markdown
## 2026-03-02 — 장운영일 체크 + Order Sync 스케줄러 최적화

- **kis/client.go**: `HolidayInfo` DTO + `GetMarketHolidayInfo()` (CTCA0903R) 추가
- **agent/market.go** (신규): `IsMarketOpen()` — KST 평일·장시간·영업일 3중 체크, 당일 캐시
- **agent/history.go**: 스케줄러 ticker에 `IsMarketOpen()` 가드 추가 (공휴일 skip)
- **api/handlers.go**: `GET /api/market/status` 핸들러 추가
- **api/router.go**: `/api/market/status` 라우트 등록
```
