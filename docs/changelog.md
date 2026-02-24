# Changelog

> Entries are in reverse chronological order (newest first).

---

## 2026-02-25

### [Fix] 모의투자 제거 및 크레덴셜 변경 시 토큰 자동 무효화

**Description:**
1. 모의투자(모의계좌 없음) 관련 코드 전체 제거
2. KIS APP KEY/SECRET 변경 시 구 토큰이 재사용되는 버그 수정 — SHA-256 fingerprint 비교로 감지 후 자동 삭제

**Files Touched:**
- `backend/cmd/server/main.go` — `InvalidateIfCredentialsChanged()` 호출 추가, mock 파라미터 제거
- `backend/internal/config/config.go` — `KISIsMock`, `KISMockURL`, `BaseURL()` 제거
- `backend/internal/kis/token.go` — `isMock`/`SetMode()` 제거; `InvalidateIfCredentialsChanged()` 추가 (settings 테이블에 credentials hash 저장); `GetCurrentToken`/`saveToken` 단순화
- `backend/internal/kis/client.go` — `isMock`/`mockBaseURL`/`realBaseURL`/`SetMock()`/`IsMock()`/`trID()` 제거; 실전 TR ID 하드코딩 (`TTTC8908R`, `TTTC8434R`, `TTTC0802U`, `TTTC0801U`, `TTTC8001R`)
- `backend/internal/models/models.go` — `Token.IsMock` 필드 제거
- `backend/internal/database/db.go` — tokens 테이블에서 `is_mock` 컬럼 제거, ALTER TABLE 마이그레이션 제거
- `backend/internal/api/handlers.go` — `SetMode` 핸들러 제거; `GetSettings` 응답에서 `is_mock` 제거
- `backend/internal/api/router.go` — `PUT /api/settings/mode` 라우트 제거
- `frontend/src/pages/Settings.jsx` — 모의/실전 토글 UI 제거; 계좌 정보만 표시

### [Fix] KIS API 잔고 조회 및 TR ID 수정

**Description:**
- `inquire-balance` output2가 배열임을 반영해 파싱 수정 → 총평가금액 정상 표시
- `inquire-psbl-order`에 필수 파라미터 `ORD_DVSN=01` 추가
- 모의투자 TR ID(`VTTC*`) 사용으로 인한 "모의투자 TR이 아닙니다" 오류 수정

**Files Touched:**
- `backend/internal/kis/client.go` — `GetInquireBalance` output2 배열 파싱, `GetAvailableOrder` ORD_DVSN=01 파라미터 추가
- `backend/internal/agent/balance.go` — `GetInquireBalance` + `GetAvailableOrder` 두 엔드포인트 조합으로 잔고 계산

### [Fix] 서버 재시작 시 KIS 토큰 재사용 (EGW00133 방지)

**Description:**
- 서버 재시작마다 토큰을 새로 발급해 KIS의 1분당 1회 제한(EGW00133)에 걸리는 문제 해결
- `EnsureToken()` 도입: DB에 유효 토큰(잔여 1시간 이상)이 있으면 재사용, 없으면 새로 발급

**Files Touched:**
- `backend/internal/kis/token.go` — `EnsureToken()` 메서드 추가
- `backend/cmd/server/main.go` — 시작 시 `IssueToken` → `EnsureToken` 변경

### [Feature] CD 파이프라인 추가 (NCP 서버 자동 배포)

**Description:**
- GitHub Actions에서 main 브랜치 push 시 NCP 서버에 자동 배포
- linux/amd64 크로스 컴파일 → SCP 전송 → React dist rsync → systemctl restart

**Files Touched:**
- `.github/workflows/ci.yml` — CD 단계 추가 (stop/transfer/restart)
- `deploy/micro-trading.service` — systemd 서비스 유닛 파일

---

## 2026-02-24

### [Feature] 초기 프로젝트 전체 구축 (Phase 1–7)

**Description:** 자동화 AI 트레이딩 시스템의 전체 초기 구조를 구축했습니다. Go 백엔드, React 프론트엔드, SQLite DB, KIS API 통합, CI/CD 파이프라인을 포함합니다.

**Files Touched:**
- `.gitignore` — Go, Node, .env, SQLite 제외 설정
- `.env.example` — 환경변수 키 목록 (실제 값 없음)
- `.github/workflows/ci.yml` — Go 빌드/테스트 + React 린트/빌드 자동화
- `backend/go.mod` — Go 모듈 (`github.com/micro-trading-for-agent/backend`)
- `backend/cmd/server/main.go` — 서버 진입점 (graceful shutdown 포함)
- `backend/internal/config/config.go` — .env 기반 설정 관리
- `backend/internal/database/db.go` — SQLite 초기화 + 자동 마이그레이션
- `backend/internal/models/models.go` — DB 모델 구조체 정의
- `backend/internal/logger/logger.go` — 구조화 JSON 로깅 (KISError 필수 필드 포함)
- `backend/internal/kis/ratelimiter.go` — KIS TPS 제한 (15 req/s)
- `backend/internal/kis/token.go` — KIS OAuth 토큰 발급 + 20시간 자동 갱신
- `backend/internal/kis/client.go` — KIS API 클라이언트 (주가/잔고/주문/내역)
- `backend/internal/agent/stock_info.go` — 종목 정보 조회
- `backend/internal/agent/balance.go` — 계좌 잔고 조회 + DB 스냅샷
- `backend/internal/agent/order.go` — 주문 실행 + DB 저장
- `backend/internal/agent/history.go` — 주문 내역 조회 + 상태 동기화
- `backend/internal/api/handlers.go` — HTTP 핸들러 (6개 엔드포인트)
- `backend/internal/api/router.go` — gin 라우터 설정
- `frontend/` — Vite+React+TailwindCSS 전체 구조
- `docs/architecture.md` — 프로젝트 아키텍처 문서
- `docs/db_schema.md` — SQLite 스키마 문서

**Pending/Next Steps:**
- `backend/go.sum` 생성 필요: `cd backend && go mod download`
- `frontend/package-lock.json` 생성 필요: `cd frontend && npm install`
- `.env` 파일 생성 후 실제 KIS API 키 입력
- KIS 모의투자 환경에서 토큰 발급 테스트
- `go test ./...` 용 단위 테스트 파일 추가 (현재 없음)
