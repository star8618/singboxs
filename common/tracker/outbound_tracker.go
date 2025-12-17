package tracker

import (
	"sync"
	"sync/atomic"
	"time"
)

// outboundTracker 追踪outbound连接状态（内部类型）
type outboundTracker struct {
	access sync.RWMutex
	
	// 每个outbound的连接统计
	stats map[string]*outboundStats
}

// outboundStats outbound统计信息（内部类型）
type outboundStats struct {
	Tag                string
	TotalAttempts      atomic.Int64 // 总尝试次数
	SuccessCount       atomic.Int64 // 成功次数
	FailureCount       atomic.Int64 // 失败次数
	LastSuccess        atomic.Int64 // 最后成功时间戳
	LastFailure        atomic.Int64 // 最后失败时间戳
	LastError          atomic.Value // 最后的错误信息 (string)
	ConsecutiveFailures atomic.Int32 // 连续失败次数
}

var globalTracker = &outboundTracker{
	stats: make(map[string]*outboundStats),
}

// getGlobalTracker 获取全局追踪器（内部函数）
func getGlobalTracker() *outboundTracker {
	return globalTracker
}

// GetOrCreateStats 获取或创建outbound统计
func (t *outboundTracker) GetOrCreateStats(tag string) *outboundStats {
	t.access.RLock()
	stats, exists := t.stats[tag]
	t.access.RUnlock()
	
	if exists {
		return stats
	}
	
	t.access.Lock()
	defer t.access.Unlock()
	
	// 双重检查
	if stats, exists := t.stats[tag]; exists {
		return stats
	}
	
	stats = &outboundStats{
		Tag: tag,
	}
	t.stats[tag] = stats
	return stats
}

// RecordSuccess 记录成功连接
func (s *outboundStats) RecordSuccess() {
	s.TotalAttempts.Add(1)
	s.SuccessCount.Add(1)
	s.LastSuccess.Store(time.Now().Unix())
	s.ConsecutiveFailures.Store(0)
}

// RecordFailure 记录失败连接
func (s *outboundStats) RecordFailure(err error) {
	s.TotalAttempts.Add(1)
	s.FailureCount.Add(1)
	s.LastFailure.Store(time.Now().Unix())
	s.ConsecutiveFailures.Add(1)
	if err != nil {
		s.LastError.Store(err.Error())
	}
}

// GetStatus 获取当前状态
// 返回: status (0=未知, 1=正常, 2=失败)
func (s *outboundStats) GetStatus() int32 {
	consecutiveFailures := s.ConsecutiveFailures.Load()
	lastSuccess := s.LastSuccess.Load()
	lastFailure := s.LastFailure.Load()
	
	// 🔥 优化：如果连续失败1次，即判定为失败（加快检测速度）
	// 第一次连接失败，基本说明节点不可用，没必要重试
	if consecutiveFailures >= 1 {
		return 2
	}
	
	// 如果有成功记录且在30秒内
	if lastSuccess > 0 && time.Now().Unix()-lastSuccess < 30 {
		return 1
	}
	
	// 如果有失败记录且在10秒内
	if lastFailure > 0 && time.Now().Unix()-lastFailure < 10 {
		return 0
	}
	
	// 未知状态
	return 0
}

// GetConsecutiveFailures 获取连续失败次数
func (s *outboundStats) GetConsecutiveFailures() int32 {
	return s.ConsecutiveFailures.Load()
}

// GetStats 获取统计信息
func (s *outboundStats) GetStats() (total, success, failure int64) {
	return s.TotalAttempts.Load(), s.SuccessCount.Load(), s.FailureCount.Load()
}

