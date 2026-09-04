package promptaudit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecisionCache_SlidingTTLRefreshesOnHit(t *testing.T) {
	now := time.Date(2026, 9, 4, 22, 0, 0, 0, time.UTC)
	cache := NewDecisionCache(8, DefaultDecisionCacheTTL)
	cache.SetClock(func() time.Time { return now })

	cache.Put("k", &Decision{Kind: DecisionAllow, AllowNextStage: true})

	now = now.Add(59 * time.Minute)
	got, hit := cache.Get("k")
	require.True(t, hit)
	require.NotNil(t, got)
	assert.Equal(t, DecisionAllow, got.Kind)

	// 命中已续期：再过 59 分钟仍命中
	now = now.Add(59 * time.Minute)
	_, hit = cache.Get("k")
	require.True(t, hit)

	// 空闲超过 TTL 后失效
	now = now.Add(61 * time.Minute)
	_, hit = cache.Get("k")
	assert.False(t, hit)
}

func TestDecisionCache_ExpiresAfterIdleTTLWithoutHit(t *testing.T) {
	now := time.Date(2026, 9, 4, 22, 0, 0, 0, time.UTC)
	cache := NewDecisionCache(8, DefaultDecisionCacheTTL)
	cache.SetClock(func() time.Time { return now })

	cache.Put("k", &Decision{Kind: DecisionAllow, AllowNextStage: true})

	now = now.Add(61 * time.Minute)
	_, hit := cache.Get("k")
	assert.False(t, hit)
}

func TestDecisionCache_ZeroTTLFallsBackToSixtyMinutes(t *testing.T) {
	now := time.Date(2026, 9, 4, 22, 0, 0, 0, time.UTC)
	cache := NewDecisionCache(8, 0)
	cache.SetClock(func() time.Time { return now })

	cache.Put("k", &Decision{Kind: DecisionAllow, AllowNextStage: true})

	// 若仍回退到旧的 10 分钟 TTL，11 分钟后会 miss
	now = now.Add(11 * time.Minute)
	_, hit := cache.Get("k")
	require.True(t, hit, "默认 TTL 应为 60 分钟，11 分钟空闲仍应命中")

	now = now.Add(61 * time.Minute)
	_, hit = cache.Get("k")
	assert.False(t, hit)
}

func TestDecisionCache_ExpiredGetDoesNotRefresh(t *testing.T) {
	now := time.Date(2026, 9, 4, 22, 0, 0, 0, time.UTC)
	cache := NewDecisionCache(8, DefaultDecisionCacheTTL)
	cache.SetClock(func() time.Time { return now })

	cache.Put("k", &Decision{Kind: DecisionAllow, AllowNextStage: true})

	now = now.Add(61 * time.Minute)
	_, hit := cache.Get("k")
	require.False(t, hit)

	_, hit = cache.Get("k")
	assert.False(t, hit)
}
