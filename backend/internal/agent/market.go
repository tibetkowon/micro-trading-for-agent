package agent

import (
	"context"
	"sync"
	"time"

	"github.com/micro-trading-for-agent/backend/internal/kis"
	"github.com/micro-trading-for-agent/backend/internal/logger"
)

var kstLocation = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		loc = time.FixedZone("KST", 9*60*60) // tzdata 없는 환경 대비
	}
	return loc
}()

// KSTLocation returns the KST (*time.Location) used for market hour calculations.
func KSTLocation() *time.Location { return kstLocation }

type marketDayCache struct {
	mu       sync.RWMutex
	date     string
	isBizDay bool
}

var pkgCache marketDayCache

// IsMarketOpen returns true when all three conditions hold:
//  1. KST weekday (Mon–Fri)
//  2. KST 09:00–15:29 (장 운영 시간)
//  3. KIS CTCA0903R 기준 영업일 (당일 캐시, KIS 호출은 하루 최대 1회)
//
// API 오류 시 false + error 반환 (fail-safe).
func IsMarketOpen(ctx context.Context, client *kis.Client) (bool, error) {
	now := time.Now().In(kstLocation)

	if wd := now.Weekday(); wd == time.Saturday || wd == time.Sunday {
		return false, nil
	}

	openMinute := now.Hour()*60 + now.Minute()
	if openMinute < 9*60 || openMinute >= 15*60+30 {
		return false, nil
	}

	today := now.Format("20060102")

	pkgCache.mu.RLock()
	if pkgCache.date == today {
		v := pkgCache.isBizDay
		pkgCache.mu.RUnlock()
		return v, nil
	}
	pkgCache.mu.RUnlock()

	info, err := client.GetMarketHolidayInfo(ctx, today)
	if err != nil {
		logger.Warn("market holiday check failed — assuming closed", map[string]any{"date": today, "error": err.Error()})
		return false, err
	}

	isBiz := info.IsBizDay == "Y"
	pkgCache.mu.Lock()
	pkgCache.date, pkgCache.isBizDay = today, isBiz
	pkgCache.mu.Unlock()

	logger.Info("market day cache refreshed", map[string]any{"date": today, "is_biz_day": isBiz})
	return isBiz, nil
}
