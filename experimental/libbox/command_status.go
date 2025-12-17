package libbox

import (
	"encoding/binary"
	"net"
	"runtime"
	"time"

	"github.com/sagernet/sing-box/common/conntrack"
	"github.com/sagernet/sing-box/common/tracker"
	"github.com/sagernet/sing-box/experimental/clashapi"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/memory"
)

type StatusMessage struct {
	Memory           int64
	Goroutines       int32
	ConnectionsIn    int32
	ConnectionsOut   int32
	TrafficAvailable bool
	Uplink           int64
	Downlink         int64
	UplinkTotal      int64
	DownlinkTotal    int64

	// 直连流量统计
	DirectUplinkTotal   int64
	DirectDownlinkTotal int64

	// 代理流量统计
	ProxyUplinkTotal   int64
	ProxyDownlinkTotal int64

	// DNS统计
	DNSTotalQueries   int64
	DNSSuccessQueries int64
	DNSCachedQueries  int64

	// 节点状态 (0=未知, 1=正常, 2=失败)
	OutboundStatus int32
	// 节点延迟 (毫秒)
	OutboundDelay int32
}

func (s *CommandServer) readStatus() StatusMessage {
	var message StatusMessage
	message.Memory = int64(memory.Inuse())
	message.Goroutines = int32(runtime.NumGoroutine())
	message.ConnectionsOut = int32(conntrack.Count())

	if s.service != nil {
		message.TrafficAvailable = true
		clashServer := s.service.clashServer.(*clashapi.Server)
		trafficManager := clashServer.TrafficManager()
		message.UplinkTotal, message.DownlinkTotal = trafficManager.Total()
		message.DirectUplinkTotal, message.DirectDownlinkTotal = trafficManager.DirectTotal()
		message.ProxyUplinkTotal, message.ProxyDownlinkTotal = trafficManager.ProxyTotal()
		message.ConnectionsIn = int32(trafficManager.ConnectionsLen())

		// 获取DNS统计
		if dnsRouter := clashServer.DNSRouter(); dnsRouter != nil {
			message.DNSTotalQueries, message.DNSSuccessQueries, message.DNSCachedQueries = dnsRouter.GetDNSStats()
		}

		// 检查当前节点状态
		message.OutboundStatus, message.OutboundDelay = s.checkOutboundStatus()
	}

	return message
}

// checkOutboundStatus 检查当前outbound的连接状态
// 返回: status (0=未知, 1=正常, 2=失败), delay (毫秒)
func (s *CommandServer) checkOutboundStatus() (int32, int32) {
	if s.service == nil || s.service.instance == nil {
		return 0, 0
	}

	// 🔥 优先检查 proxy-main 的状态（这是主要的代理出站）
	// 如果 proxy-main 不存在，才回退到默认 outbound
	var outboundTag string
	outboundManager := s.service.instance.Outbound()

	// 尝试获取 proxy-main
	if proxyMain, exists := outboundManager.Outbound("proxy-main"); exists && proxyMain != nil {
		outboundTag = "proxy-main"
	} else {
		// 回退到默认 outbound
		defaultOutbound := outboundManager.Default()
		if defaultOutbound == nil {
			return 0, 0
		}
		outboundTag = defaultOutbound.Tag()
	}

	// 🔥 直接使用tracker统计
	total, success, failure := tracker.GetOutboundStats(outboundTag)
	consecutiveFailures := tracker.GetConsecutiveFailures(outboundTag)

	var status int32 = 0
	var delay int32 = 0

	// 判断状态
	if total == 0 {
		// 没有连接记录，未知状态
		status = 0
	} else if consecutiveFailures >= 1 {
		// 🔥 优化：第1次失败即判定失败（加快检测速度到2-3秒内）
		// 如果节点第一次连接就失败，基本说明节点不可用，没必要重试
		status = 2
	} else if success > 0 && failure == 0 {
		// 有成功无失败，正常状态
		status = 1
	} else if success > failure {
		// 成功多于失败，正常状态
		status = 1
	} else {
		// 其他情况，未知状态
		status = 0
	}

	return status, delay
}

func (s *CommandServer) handleStatusConn(conn net.Conn) error {
	var interval int64
	err := binary.Read(conn, binary.BigEndian, &interval)
	if err != nil {
		return E.Cause(err, "read interval")
	}
	ticker := time.NewTicker(time.Duration(interval))
	defer ticker.Stop()
	ctx := connKeepAlive(conn)
	status := s.readStatus()
	uploadTotal := status.UplinkTotal
	downloadTotal := status.DownlinkTotal
	for {
		err = binary.Write(conn, binary.BigEndian, status)
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
		status = s.readStatus()
		upload := status.UplinkTotal - uploadTotal
		download := status.DownlinkTotal - downloadTotal
		uploadTotal = status.UplinkTotal
		downloadTotal = status.DownlinkTotal
		status.Uplink = upload
		status.Downlink = download
	}
}

func (c *CommandClient) handleStatusConn(conn net.Conn) {
	for {
		var message StatusMessage
		err := binary.Read(conn, binary.BigEndian, &message)
		if err != nil {
			c.handler.Disconnected(err.Error())
			return
		}
		c.handler.WriteStatus(&message)
	}
}
