package agent

import (
	"context"

	"github.com/micro-trading-for-agent/backend/internal/kis"
)

// GetVolumeRank returns the volume ranking (거래량 순위).
// market: "J"=KRX(default). sort: "0"=평균거래량, "1"=거래량증가율, "2"=거래회전율, "3"=거래대금순.
// priceMin/priceMax: 가격 범위 (빈값="" 이면 전체).
// excludeCls: fid_trgt_exls_cls_code 10자리 문자열 (빈값이면 "1111111111" 사용).
func GetVolumeRank(ctx context.Context, client *kis.Client, market, sort, priceMin, priceMax, excludeCls string) ([]kis.VolumeRankItem, error) {
	if market == "" {
		market = "J"
	}
	if sort == "" {
		sort = "0"
	}
	return client.GetVolumeRank(ctx, market, sort, priceMin, priceMax, excludeCls)
}

// GetStrengthRank returns the execution strength ranking (체결강도 상위).
// market: "0000"=전체(default), "0001"=거래소, "1001"=코스닥, "2001"=코스피200.
// priceMin/priceMax: 가격 범위 (빈값="" 이면 전체).
// excludeCls: fid_trgt_exls_cls_code 10자리 문자열 (빈값이면 "1111111111" 사용).
func GetStrengthRank(ctx context.Context, client *kis.Client, market, priceMin, priceMax, excludeCls string) ([]kis.StrengthRankItem, error) {
	if market == "" {
		market = "0000"
	}
	return client.GetStrengthRank(ctx, market, priceMin, priceMax, excludeCls)
}

// GetExecCountRank returns the bulk execution count ranking (대량체결건수 상위).
// market: "0000"=전체(default). sort: "0"=매수상위(default), "1"=매도상위.
// priceMin/priceMax: 가격 범위 (빈값="" 이면 전체).
// excludeCls: fid_trgt_exls_cls_code 10자리 문자열 (빈값이면 "1111111111" 사용).
func GetExecCountRank(ctx context.Context, client *kis.Client, market, sort, priceMin, priceMax, excludeCls string) ([]kis.ExecCountRankItem, error) {
	if market == "" {
		market = "0000"
	}
	if sort == "" {
		sort = "0"
	}
	return client.GetExecCountRank(ctx, market, sort, priceMin, priceMax, excludeCls)
}

// GetDisparityRank returns the disparity index ranking (이격도 순위).
// market: "0000"=전체(default). period: "5","10","20"(default),"60","120". sort: "0"=상위(default), "1"=하위.
// priceMin/priceMax: 가격 범위 (빈값="" 이면 전체).
// excludeCls: fid_trgt_exls_cls_code 10자리 문자열 (빈값이면 "1111111111" 사용).
func GetDisparityRank(ctx context.Context, client *kis.Client, market, period, sort, priceMin, priceMax, excludeCls string) ([]kis.DisparityRankItem, error) {
	if market == "" {
		market = "0000"
	}
	if period == "" {
		period = "20"
	}
	if sort == "" {
		sort = "0"
	}
	return client.GetDisparityRank(ctx, market, period, sort, priceMin, priceMax, excludeCls)
}
