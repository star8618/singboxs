//go:build linux

package redirect

import (
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strings"

	"github.com/sagernet/sing-box/log"
)

const (
	PROXY_FWMARK      = "0x2d0"
	PROXY_ROUTE_TABLE = "0x2d0"
)

type TProxyIPTables struct {
	logger        log.ContextLogger
	tproxyPort    uint16
	dnsPort       uint16
	interfaceName string
	bypass        []string
	dnsRedirect   bool
	routingMark   uint32
	enabled       bool
}

func NewTProxyIPTables(logger log.ContextLogger) *TProxyIPTables {
	return &TProxyIPTables{
		logger: logger,
	}
}

func (t *TProxyIPTables) Setup(interfaceName string, bypass []string, tproxyPort uint16, dnsRedirect bool, dnsPort uint16, routingMark uint32) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("auto_iptables only supports Linux")
	}

	if _, err := exec.LookPath("iptables"); err != nil {
		return fmt.Errorf("iptables command not found: %w", err)
	}

	if interfaceName == "" {
		interfaceName = "lo"
	}

	t.interfaceName = interfaceName
	t.tproxyPort = tproxyPort
	t.dnsPort = dnsPort
	t.bypass = bypass
	t.dnsRedirect = dnsRedirect
	t.routingMark = routingMark
	if t.routingMark == 0 {
		t.routingMark = 2158
	}

	// add route
	t.execCmd(fmt.Sprintf("ip -f inet rule add fwmark %s lookup %s", PROXY_FWMARK, PROXY_ROUTE_TABLE))
	t.execCmd(fmt.Sprintf("ip -f inet route add local default dev %s table %s", interfaceName, PROXY_ROUTE_TABLE))

	// set FORWARD
	if interfaceName != "lo" {
		t.execCmd("sysctl -w net.ipv4.ip_forward=1")
		t.execCmd(fmt.Sprintf("iptables -t filter -A FORWARD -o %s -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT", interfaceName))
		t.execCmd(fmt.Sprintf("iptables -t filter -A FORWARD -o %s -j ACCEPT", interfaceName))
		t.execCmd(fmt.Sprintf("iptables -t filter -A FORWARD -i %s ! -o %s -j ACCEPT", interfaceName, interfaceName))
		t.execCmd(fmt.Sprintf("iptables -t filter -A FORWARD -i %s -o %s -j ACCEPT", interfaceName, interfaceName))
	}

	// set sing-box divert
	t.execCmd("iptables -t mangle -N sing_box_divert")
	t.execCmd("iptables -t mangle -F sing_box_divert")
	t.execCmd(fmt.Sprintf("iptables -t mangle -A sing_box_divert -j MARK --set-mark %s", PROXY_FWMARK))
	t.execCmd("iptables -t mangle -A sing_box_divert -j ACCEPT")

	// set pre routing
	t.execCmd("iptables -t mangle -N sing_box_prerouting")
	t.execCmd("iptables -t mangle -F sing_box_prerouting")
	t.execCmd("iptables -t mangle -A sing_box_prerouting -s 172.17.0.0/16 -j RETURN")
	if t.dnsRedirect {
		t.execCmd("iptables -t mangle -A sing_box_prerouting -p udp --dport 53 -j ACCEPT")
		t.execCmd("iptables -t mangle -A sing_box_prerouting -p tcp --dport 53 -j ACCEPT")
	}
	t.execCmd("iptables -t mangle -A sing_box_prerouting -m addrtype --dst-type LOCAL -j RETURN")
	t.addLocalnetworkToChain("sing_box_prerouting")
	t.execCmd("iptables -t mangle -A sing_box_prerouting -p tcp -m socket -j sing_box_divert")
	t.execCmd("iptables -t mangle -A sing_box_prerouting -p udp -m socket -j sing_box_divert")
	t.execCmd(fmt.Sprintf("iptables -t mangle -A sing_box_prerouting -p tcp -j TPROXY --on-port %d --tproxy-mark %s/%s", tproxyPort, PROXY_FWMARK, PROXY_FWMARK))
	t.execCmd(fmt.Sprintf("iptables -t mangle -A sing_box_prerouting -p udp -j TPROXY --on-port %d --tproxy-mark %s/%s", tproxyPort, PROXY_FWMARK, PROXY_FWMARK))
	t.execCmd("iptables -t mangle -A PREROUTING -j sing_box_prerouting")

	if t.dnsRedirect && t.dnsPort > 0 {
		t.execCmd(fmt.Sprintf("iptables -t nat -I PREROUTING ! -s 172.17.0.0/16 ! -d 127.0.0.0/8 -p tcp --dport 53 -j REDIRECT --to %d", dnsPort))
		t.execCmd(fmt.Sprintf("iptables -t nat -I PREROUTING ! -s 172.17.0.0/16 ! -d 127.0.0.0/8 -p udp --dport 53 -j REDIRECT --to %d", dnsPort))
	}

	// set post routing
	if interfaceName != "lo" {
		t.execCmd(fmt.Sprintf("iptables -t nat -A POSTROUTING -o %s -m addrtype ! --src-type LOCAL -j MASQUERADE", interfaceName))
	}

	// set output
	t.execCmd("iptables -t mangle -N sing_box_output")
	t.execCmd("iptables -t mangle -F sing_box_output")
	t.execCmd(fmt.Sprintf("iptables -t mangle -A sing_box_output -m mark --mark %#x -j RETURN", t.routingMark))
	if t.dnsRedirect {
		t.execCmd("iptables -t mangle -A sing_box_output -p udp -m multiport --dports 53,123,137 -j ACCEPT")
		t.execCmd("iptables -t mangle -A sing_box_output -p tcp --dport 53 -j ACCEPT")
	}
	t.execCmd("iptables -t mangle -A sing_box_output -m addrtype --dst-type LOCAL -j RETURN")
	t.execCmd("iptables -t mangle -A sing_box_output -m addrtype --dst-type BROADCAST -j RETURN")
	t.addLocalnetworkToChain("sing_box_output")
	t.execCmd(fmt.Sprintf("iptables -t mangle -A sing_box_output -p tcp -j MARK --set-mark %s", PROXY_FWMARK))
	t.execCmd(fmt.Sprintf("iptables -t mangle -A sing_box_output -p udp -j MARK --set-mark %s", PROXY_FWMARK))
	t.execCmd(fmt.Sprintf("iptables -t mangle -I OUTPUT -o %s -j sing_box_output", interfaceName))

	// set dns output
	if t.dnsRedirect && t.dnsPort > 0 {
		t.execCmd("iptables -t nat -N sing_box_dns_output")
		t.execCmd("iptables -t nat -F sing_box_dns_output")
		t.execCmd(fmt.Sprintf("iptables -t nat -A sing_box_dns_output -m mark --mark %#x -j RETURN", t.routingMark))
		t.execCmd("iptables -t nat -A sing_box_dns_output -s 172.17.0.0/16 -j RETURN")
		t.execCmd(fmt.Sprintf("iptables -t nat -A sing_box_dns_output -p udp -j REDIRECT --to-ports %d", dnsPort))
		t.execCmd(fmt.Sprintf("iptables -t nat -A sing_box_dns_output -p tcp -j REDIRECT --to-ports %d", dnsPort))
		t.execCmd("iptables -t nat -I OUTPUT -p tcp --dport 53 -j sing_box_dns_output")
		t.execCmd("iptables -t nat -I OUTPUT -p udp --dport 53 -j sing_box_dns_output")
	}

	t.enabled = true
	t.logger.Info("[IPTABLES] setting iptables completed")
	return nil
}

func (t *TProxyIPTables) Cleanup() {
	if !t.enabled || runtime.GOOS != "linux" || t.interfaceName == "" || t.tproxyPort == 0 {
		return
	}

	t.logger.Warn("[IPTABLES] cleanup tproxy iptables")

	// check if chain exists
	if _, err := t.execCmdWithOutput("iptables -t mangle -L sing_box_divert"); err != nil {
		return
	}

	// clean route
	t.execCmd(fmt.Sprintf("ip -f inet rule del fwmark %s lookup %s", PROXY_FWMARK, PROXY_ROUTE_TABLE))
	t.execCmd(fmt.Sprintf("ip -f inet route del local default dev %s table %s", t.interfaceName, PROXY_ROUTE_TABLE))

	// clean FORWARD
	if t.interfaceName != "lo" {
		t.execCmd(fmt.Sprintf("iptables -t filter -D FORWARD -i %s ! -o %s -j ACCEPT", t.interfaceName, t.interfaceName))
		t.execCmd(fmt.Sprintf("iptables -t filter -D FORWARD -i %s -o %s -j ACCEPT", t.interfaceName, t.interfaceName))
		t.execCmd(fmt.Sprintf("iptables -t filter -D FORWARD -o %s -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT", t.interfaceName))
		t.execCmd(fmt.Sprintf("iptables -t filter -D FORWARD -o %s -j ACCEPT", t.interfaceName))
	}

	// clean PREROUTING
	if t.dnsRedirect && t.dnsPort > 0 {
		t.execCmd(fmt.Sprintf("iptables -t nat -D PREROUTING ! -s 172.17.0.0/16 ! -d 127.0.0.0/8 -p tcp --dport 53 -j REDIRECT --to %d", t.dnsPort))
		t.execCmd(fmt.Sprintf("iptables -t nat -D PREROUTING ! -s 172.17.0.0/16 ! -d 127.0.0.0/8 -p udp --dport 53 -j REDIRECT --to %d", t.dnsPort))
	}
	t.execCmd("iptables -t mangle -D PREROUTING -j sing_box_prerouting")

	// clean POSTROUTING
	if t.interfaceName != "lo" {
		t.execCmd(fmt.Sprintf("iptables -t nat -D POSTROUTING -o %s -m addrtype ! --src-type LOCAL -j MASQUERADE", t.interfaceName))
	}

	// clean OUTPUT
	t.execCmd(fmt.Sprintf("iptables -t mangle -D OUTPUT -o %s -j sing_box_output", t.interfaceName))
	if t.dnsRedirect && t.dnsPort > 0 {
		t.execCmd("iptables -t nat -D OUTPUT -p tcp --dport 53 -j sing_box_dns_output")
		t.execCmd("iptables -t nat -D OUTPUT -p udp --dport 53 -j sing_box_dns_output")
	}

	// clean chains
	t.execCmd("iptables -t mangle -F sing_box_prerouting")
	t.execCmd("iptables -t mangle -X sing_box_prerouting")
	t.execCmd("iptables -t mangle -F sing_box_divert")
	t.execCmd("iptables -t mangle -X sing_box_divert")
	t.execCmd("iptables -t mangle -F sing_box_output")
	t.execCmd("iptables -t mangle -X sing_box_output")
	if t.dnsRedirect && t.dnsPort > 0 {
		t.execCmd("iptables -t nat -F sing_box_dns_output")
		t.execCmd("iptables -t nat -X sing_box_dns_output")
	}

	t.enabled = false
	t.interfaceName = ""
	t.tproxyPort = 0
	t.dnsPort = 0
}

func (t *TProxyIPTables) addLocalnetworkToChain(chain string) {
	// add user bypass networks first (same as mihomo)
	for _, network := range t.bypass {
		if _, _, err := net.ParseCIDR(network); err != nil {
			t.logger.Warn("[IPTABLES] invalid bypass network: ", network, " error: ", err)
			continue
		}
		t.execCmd(fmt.Sprintf("iptables -t mangle -A %s -d %s -j RETURN", chain, network))
	}

	// default bypass networks
	localNetworks := []string{
		"0.0.0.0/8",
		"10.0.0.0/8",
		"100.64.0.0/10",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"172.16.0.0/12",
		"192.0.0.0/24",
		"192.0.2.0/24",
		"192.88.99.0/24",
		"192.168.0.0/16",
		"198.18.0.0/15",
		"198.51.100.0/24",
		"203.0.113.0/24",
		"224.0.0.0/4",
		"240.0.0.0/4",
		"255.255.255.255/32",
	}

	// add default local networks
	for _, network := range localNetworks {
		t.execCmd(fmt.Sprintf("iptables -t mangle -A %s -d %s -j RETURN", chain, network))
	}
}

func (t *TProxyIPTables) execCmd(command string) {
	t.logger.Debug("[IPTABLES] ", command)
	args := strings.Fields(command)
	if len(args) == 0 {
		return
	}
	cmd := exec.Command(args[0], args[1:]...)
	_ = cmd.Run()
}

func (t *TProxyIPTables) execCmdWithOutput(command string) (string, error) {
	args := strings.Fields(command)
	if len(args) == 0 {
		return "", fmt.Errorf("empty command")
	}
	cmd := exec.Command(args[0], args[1:]...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}
