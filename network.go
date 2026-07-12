package main

import (
	_ "embed"
	"fmt"
	"net"
	"os"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

//go:embed templates/networking/dynamic
var dynamicNetworkTemplate string

//go:embed templates/networking/static
var staticNetworkTemplate string

// svDir and runsvdirCurrent describe the runit layout void uses to
// enable/disable services: a service is enabled by symlinking its
// definition in /etc/sv into the active runsvdir.
const (
	svDir           = "/etc/sv"
	runsvdirCurrent = "/etc/runit/runsvdir/current"
	dhcpcdConfPath  = "/etc/dhcpcd.conf"
	rcLocalPath     = "/etc/rc.local"
	resolvConfPath  = "/etc/resolv.conf"
)

// NetworkConfig covers the cloud-init NoCloud network-config v1 format
// Proxmox writes alongside user-data.
type NetworkConfig struct {
	Version int                   `yaml:"version"`
	Config  []NetworkConfigDevice `yaml:"config"`
}

// NetworkConfigDevice describes a single physical network interface and
// the subnets attached to it.
type NetworkConfigDevice struct {
	Type    string   `yaml:"type"`
	Name    string   `yaml:"name"`
	Subnets []Subnet `yaml:"subnets"`
}

// Subnet describes one address assignment for an interface: either "dhcp"
// or "static" with an address/netmask/gateway.
type Subnet struct {
	Type           string   `yaml:"type"`
	Address        string   `yaml:"address"`
	Netmask        string   `yaml:"netmask"`
	Gateway        string   `yaml:"gateway"`
	DNSNameservers []string `yaml:"dns_nameservers"`
	DNSSearch      []string `yaml:"dns_search"`
}

// staticIPTemplateData is the set of values substituted into the
// static-ip template.
type staticIPTemplateData struct {
	Interface string
	Address   string
	CIDR      int
	Gateway   string
}

// ParseNetworkConfig parses raw cloud-init network-config content.
func ParseNetworkConfig(data []byte) (*NetworkConfig, error) {
	var networkConfig NetworkConfig
	if err := yaml.Unmarshal(data, &networkConfig); err != nil {
		return nil, fmt.Errorf("parse network-config: %w", err)
	}

	return &networkConfig, nil
}

// ApplyNetworkConfig renders and applies the networking setup for every
// interface in the config: DHCP interfaces enable dhcpcd via runit and get
// /etc/dhcpcd.conf, static interfaces disable dhcpcd and get their
// addressing applied through /etc/rc.local.
func ApplyNetworkConfig(nc *NetworkConfig) error {
	for _, device := range nc.Config {
		if device.Type != "physical" {
			continue
		}

		for _, subnet := range device.Subnets {
			switch subnet.Type {
			case "dhcp", "dhcp4", "dhcp6", "ipv6_slaac", "ipv6_dhcpv6-stateless", "ipv6_dhcpv6-stateful":
				if err := applyDynamicNetwork(); err != nil {
					return err
				}
			case "static", "static6":
				if err := applyStaticNetwork(device.Name, subnet); err != nil {
					return err
				}
			default:
				return fmt.Errorf("unsupported subnet type %q for interface %s", subnet.Type, device.Name)
			}
		}
	}

	return nil
}

// applyDynamicNetwork renders dhcpcd.conf and enables the dhcpcd service.
func applyDynamicNetwork() error {
	if err := os.WriteFile(dhcpcdConfPath, []byte(dynamicNetworkTemplate), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", dhcpcdConfPath, err)
	}

	return enableService("dhcpcd")
}

// applyStaticNetwork renders /etc/rc.local for a statically addressed
// interface and disables the dhcpcd service.
func applyStaticNetwork(iface string, subnet Subnet) error {
	cidr, err := netmaskToCIDR(subnet.Netmask)
	if err != nil {
		return fmt.Errorf("interface %s: %w", iface, err)
	}

	tmpl, err := template.New("static-ip").Parse(staticNetworkTemplate)
	if err != nil {
		return fmt.Errorf("parse static-ip template: %w", err)
	}

	var rendered strings.Builder
	data := staticIPTemplateData{
		Interface: iface,
		Address:   subnet.Address,
		CIDR:      cidr,
		Gateway:   subnet.Gateway,
	}
	if err := tmpl.Execute(&rendered, data); err != nil {
		return fmt.Errorf("render static-ip template: %w", err)
	}

	if err := os.WriteFile(rcLocalPath, []byte(rendered.String()), 0o755); err != nil {
		return fmt.Errorf("write %s: %w", rcLocalPath, err)
	}

	if err := disableService("dhcpcd"); err != nil {
		return err
	}

	return applyResolvConf(subnet.DNSNameservers, subnet.DNSSearch)
}

// applyResolvConf writes /etc/resolv.conf for a statically addressed
// interface, since dhcpcd (which normally manages it) is disabled.
func applyResolvConf(nameservers, search []string) error {
	if len(nameservers) == 0 {
		return nil
	}

	var sb strings.Builder
	if len(search) > 0 {
		fmt.Fprintf(&sb, "search %s\n", strings.Join(search, " "))
	}
	for _, ns := range nameservers {
		fmt.Fprintf(&sb, "nameserver %s\n", ns)
	}

	if err := os.WriteFile(resolvConfPath, []byte(sb.String()), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", resolvConfPath, err)
	}

	return nil
}

// enableService symlinks a service's /etc/sv definition into the active
// runsvdir, mirroring `ln -s /etc/sv/<name> /etc/runit/runsvdir/current/`.
func enableService(name string) error {
	link := runsvdirCurrent + "/" + name
	if _, err := os.Lstat(link); err == nil {
		return nil
	}

	if err := os.Symlink(svDir+"/"+name, link); err != nil {
		return fmt.Errorf("enable service %s: %w", name, err)
	}

	return nil
}

// disableService removes a service's symlink from the active runsvdir,
// mirroring `rm /etc/runit/runsvdir/current/<name>`.
func disableService(name string) error {
	link := runsvdirCurrent + "/" + name
	if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("disable service %s: %w", name, err)
	}

	return nil
}

// netmaskToCIDR converts a dotted-decimal netmask (e.g. "255.255.255.0")
// into its CIDR prefix length (e.g. 24).
func netmaskToCIDR(netmask string) (int, error) {
	ip := net.ParseIP(netmask)
	if ip == nil {
		return 0, fmt.Errorf("invalid netmask %q", netmask)
	}

	ip4 := ip.To4()
	if ip4 == nil {
		return 0, fmt.Errorf("invalid IPv4 netmask %q", netmask)
	}

	mask := net.IPMask(ip4)
	size, bits := mask.Size()
	if bits == 0 {
		return 0, fmt.Errorf("invalid netmask %q", netmask)
	}

	return size, nil
}
