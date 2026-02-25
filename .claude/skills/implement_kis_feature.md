# Skill: KIS API 기능 구현 및 개선

## Description
KIS API 기능을 구현하거나 개선할 때, `docs/kis-api/` 의 공식 명세서를
먼저 참조하여 올바른 Request/Response 구조로 구현하도록 보장한다.

## Trigger
- KIS API 신규 엔드포인트 구현 요청 시
- 기존 KIS API 관련 코드 수정/개선 요청 시

## Instructions
1. **관련 문서 파악:** 구현할 기능과 관련된 문서를 선택한다.
   - 시세/차트/분봉/일봉 관련 → `docs/kis-api/기본시세.md`
   - 거래량/등락률/호가잔량 순위 → `docs/kis-api/순위분석.md`
   - 종목 검색/종목 기본정보 → `docs/kis-api/종목정보.md`
   - 주문(매수/매도)/계좌/잔고/주문조회 → `docs/kis-api/주문계좌.md`

2. **문서 읽기:** 해당 `.md` 파일을 Read 도구로 열어 대상 API 섹션을 찾는다.

3. **스펙 확인:** 다음 항목을 반드시 확인한다.
   - `TR_ID` (거래 ID)
   - `Method` / `URL`
   - 요청 헤더 (Header) 필드
   - 요청 파라미터 (Query/Body) 필드 및 필수 여부
   - 응답 필드 및 타입

4. **구현:** 기존 패턴을 따라 구현한다.
   - KIS 클라이언트: `backend/internal/kis/client.go` 패턴 참고
   - Rate Limiter: `c.rateLimiter.Wait(ctx)` 반드시 호출
   - 에러 로깅: `c.logAPIError()` 사용하여 `kis_api_logs` 테이블에 기록
   - 응답 파싱: `rt_cd == "0"` 으로 성공 여부 확인

5. **검증:** `verify_implementation.md` 스킬로 빌드/테스트 확인
