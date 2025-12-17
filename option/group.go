package option

import "github.com/sagernet/sing/common/json/badoption"

type SelectorOutboundOptions struct {
	Outbounds                 []string `json:"outbounds"`
	Default                   string   `json:"default,omitempty"`
	InterruptExistConnections bool     `json:"interrupt_exist_connections,omitempty"`
}

type URLTestOutboundOptions struct {
	Outbounds                 []string           `json:"outbounds"`
	URL                       string             `json:"url,omitempty"`
	Interval                  badoption.Duration `json:"interval,omitempty"`
	Tolerance                 uint16             `json:"tolerance,omitempty"`
	IdleTimeout               badoption.Duration `json:"idle_timeout,omitempty"`
	InterruptExistConnections bool               `json:"interrupt_exist_connections,omitempty"`
}

// 🔥 Failover 故障转移出站配置（基于真实连接检测）
type FailoverOutboundOptions struct {
	Outbounds                 []string           `json:"outbounds"`
	MaxFailures               int                `json:"max_failures,omitempty"`
	RecoveryInterval          badoption.Duration `json:"recovery_interval,omitempty"`
	RecoveryURL               string             `json:"recovery_url,omitempty"`
	InterruptExistConnections bool               `json:"interrupt_exist_connections,omitempty"`
}
