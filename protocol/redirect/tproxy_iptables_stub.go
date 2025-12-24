//go:build !linux

package redirect

import (
	"github.com/sagernet/sing-box/log"
)

type TProxyIPTables struct{}

func NewTProxyIPTables(logger log.ContextLogger) *TProxyIPTables {
	return &TProxyIPTables{}
}

func (t *TProxyIPTables) Setup(interfaceName string, bypass []string, tproxyPort uint16, dnsRedirect bool, dnsPort uint16, routingMark uint32) error {
	return nil
}

func (t *TProxyIPTables) Cleanup() {}
