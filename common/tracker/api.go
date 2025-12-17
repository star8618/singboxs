package tracker

import "log"

// RecordOutboundSuccess 记录outbound连接成功
func RecordOutboundSuccess(tag string) {
	stats := getGlobalTracker().GetOrCreateStats(tag)
	stats.RecordSuccess()
	log.Printf("✅ [Tracker] %s 连接成功", tag)
}

// RecordOutboundFailure 记录outbound连接失败
func RecordOutboundFailure(tag string, err error) {
	stats := getGlobalTracker().GetOrCreateStats(tag)
	stats.RecordFailure(err)
	consecutive := stats.GetConsecutiveFailures()
	log.Printf("❌ [Tracker] %s 连接失败 (连续: %d, 错误: %v)", tag, consecutive, err)
}

// GetOutboundStatus 获取outbound状态
func GetOutboundStatus(tag string) int32 {
	stats := getGlobalTracker().GetOrCreateStats(tag)
	return stats.GetStatus()
}

// GetConsecutiveFailures 获取连续失败次数
func GetConsecutiveFailures(tag string) int32 {
	stats := getGlobalTracker().GetOrCreateStats(tag)
	return stats.GetConsecutiveFailures()
}

// GetOutboundStats 获取outbound统计信息
func GetOutboundStats(tag string) (total, success, failure int64) {
	stats := getGlobalTracker().GetOrCreateStats(tag)
	return stats.GetStats()
}

