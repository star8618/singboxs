package platform

import (
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/process"
	"github.com/sagernet/sing-box/option"
	tun "github.com/sagernet/sing-tun"
	"github.com/sagernet/sing/common/logger"
)

type Interface interface {
	Initialize(networkManager adapter.NetworkManager) error
	UsePlatformAutoDetectInterfaceControl() bool
	AutoDetectInterfaceControl(fd int) error
	OpenTun(options *tun.Options, platformOptions option.TunPlatformOptions) (tun.Tun, error)
	CreateDefaultInterfaceMonitor(logger logger.Logger) tun.DefaultInterfaceMonitor
	Interfaces() ([]adapter.NetworkInterface, error)
	UnderNetworkExtension() bool
	IncludeAllNetworks() bool
	ClearDNSCache()
	ReadWIFIState() adapter.WIFIState
	SystemCertificates() []string
	process.Searcher
	SendNotification(notification *Notification) error
	// 🔥 新增：节点切换回调
	OnNodeSwitched(fromNode, toNode string)
	// 🔥 新增：所有节点失败回调
	OnAllNodesFailed()
}

type Notification struct {
	Identifier string
	TypeName   string
	TypeID     int32
	Title      string
	Subtitle   string
	Body       string
	OpenURL    string
}
