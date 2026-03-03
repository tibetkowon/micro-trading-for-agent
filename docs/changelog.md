# Changelog

## 2026-03-03 — KIS inquire-daily-ccld API 파라미터 버그 수정

- **kis/client.go**: `GetOrderHistory()` 쿼리에 누락된 필수 파라미터 3개 추가 (`INQR_DVSN_1`, `INQR_DVSN_3`, `EXCG_ID_DVSN_CD=ALL`)
- **kis/client.go**: 스펙에 없는 `CANC_YN` 파라미터 제거 — API 게이트웨이가 HTML 오류 반환하던 원인
- **kis/client.go**: `get()` 헬퍼에 `Content-Type: application/json; charset=utf-8` 헤더 추가 (GET 요청에도 명세 required)
- **영향**: 체결 완료 거래가 대기(PENDING) 상태로 표시되던 현상 해결

## 2026-03-02 — 장운영일 체크 + Order Sync 스케줄러 최적화

- **KIS client**: `HolidayInfo` DTO 및 `GetMarketHolidayInfo()` 메서드 추가 (`CTCA0903R`)
- **agent/market.go** (신규): `IsMarketOpen()` 함수 — KST 평일·장 운영 시간·KIS 영업일 3중 체크, 당일 메모리 캐시로 KIS 호출 하루 1회 제한
- **agent/history.go**: `StartOrderSyncScheduler` ticker 블록에 `IsMarketOpen()` 가드 추가 — 장 마감·공휴일 시 sync skip
- **api/handlers.go**: `GET /api/market/status` 핸들러 추가 (reason 필드: open/weekend/outside_hours/holiday/check_failed)
- **api/router.go**: `/api/market/status` 라우트 등록
- **main.go**: `import _ "time/tzdata"` 추가 — NCP Micro tzdata 미설치 환경 대비
