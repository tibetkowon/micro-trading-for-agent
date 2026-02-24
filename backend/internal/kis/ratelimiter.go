package kis

import (
	"context"

	"golang.org/x/time/rate"
)

// RateLimiter wraps golang.org/x/time/rate to enforce KIS API TPS limits.
// KIS allows up to 20 requests/second for most endpoints.
type RateLimiter struct {
	limiter *rate.Limiter
}

// NewRateLimiter creates a limiter allowing `rps` requests per second
// with a burst of `burst` (set burst == rps for strict control).
func NewRateLimiter(rps, burst int) *RateLimiter {
	return &RateLimiter{
		limiter: rate.NewLimiter(rate.Limit(rps), burst),
	}
}

// Wait blocks until a request token is available or ctx is cancelled.
func (rl *RateLimiter) Wait(ctx context.Context) error {
	return rl.limiter.Wait(ctx)
}
