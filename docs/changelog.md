# Changelog

> Entries are in reverse chronological order (newest first).

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
