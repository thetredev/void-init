package main

import (
	_ "embed"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed templates/dhcpcd
var dynamicNetworkTemplate string

// svDir and runsvdirCurrent describe the runit layout void uses to
// enable/disable services: a service is enabled by symlinking its
// definition in /etc/sv into the active runsvdir.
const (
	svDir           = "/etc/sv"
	runsvdirCurrent = "/etc/runit/runsvdir/current"
	dhcpcdConfPath  = "/etc/dhcpcd.conf"
	resolvConfPath  = "/etc/resolv.conf"
)

// NetworkConfig covers the cloud-init NoCloud network-config v1 format
// Proxmox writes alongside user-data.
type NetworkConfig struct {
	Version int                   `yaml:"version"`
	Config  []NetworkConfigDevice `yaml:"config"`
}

// NetworkConfigDevice describes one entry of the network-config "config"
// list. Depending on Type, it is either a physical network interface with
// subnets attached to it (Type == "physical"), or a global nameserver
// entry (Type == "nameserver") carrying its own Address/Search lists.
type NetworkConfigDevice struct {
	Type       string   `yaml:"type"`
	Name       string   `yaml:"name"`
	MacAddress string   `yaml:"mac_address"`
	Subnets    []Subnet `yaml:"subnets"`
	Address    []string `yaml:"address"`
	Search     []string `yaml:"search"`
}

// Subnet describes one address assignment for an interface: either "dhcp"
// or "static" with an address/gateway. The address may carry its own CIDR
// prefix (e.g. "fd8c::1/64"), in which case Netmask is left empty.
type Subnet struct {
	Type           string   `yaml:"type"`
	Address        string   `yaml:"address"`
	Netmask        string   `yaml:"netmask"`
	Gateway        string   `yaml:"gateway"`
	DNSNameservers []string `yaml:"dns_nameservers"`
	DNSSearch      []string `yaml:"dns_search"`
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
// addressing applied directly via `ip`. Nameservers gathered from static
// subnets and any top-level "nameserver" entries are merged into a single
// /etc/resolv.conf write at the end.
func ApplyNetworkConfig(nc *NetworkConfig) error {
	var nameservers, search []string

	for _, device := range nc.Config {
		switch device.Type {
		case "physical":
			for _, subnet := range device.Subnets {
				switch subnet.Type {
				case "dhcp", "dhcp4", "dhcp6", "ipv6_slaac", "ipv6_dhcpv6-stateless", "ipv6_dhcpv6-stateful":
					if err := applyDynamicNetwork(device.Name); err != nil {
						return err
					}
				case "static", "static6":
					if err := applyStaticNetwork(device.Name, subnet); err != nil {
						return err
					}
					nameservers = append(nameservers, subnet.DNSNameservers...)
					search = append(search, subnet.DNSSearch...)
				default:
					return fmt.Errorf("unsupported subnet type %q for interface %s", subnet.Type, device.Name)
				}
			}
		case "nameserver":
			nameservers = append(nameservers, device.Address...)
			search = append(search, device.Search...)
		default:
			return fmt.Errorf("unsupported config type %q", device.Type)
		}
	}

	return applyResolvConf(nameservers, search)
}

// applyDynamicNetwork brings iface up, renders dhcpcd.conf, and enables the
// dhcpcd service. dhcpcd only manages interfaces that are already up (it
// won't bring one up itself), and it also drives IPv6 SLAAC/RA handling, so
// this covers dhcp as well as ipv6_slaac/dhcpv6 subnet types.
func applyDynamicNetwork(iface string) error {
	cmd := exec.Command("ip", "link", "set", "dev", iface, "up")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ip link set dev %s up: %w: %s", iface, err, output)
	}

	if err := writeManagedFile(dhcpcdConfPath, dynamicNetworkTemplate, 0o644); err != nil {
		return err
	}

	return enableService("dhcpcd")
}

// applyStaticNetwork brings up iface with the given static addressing via
// `ip` and disables the dhcpcd service. void-init is expected to run out of
// /etc/rc.local, which runit executes before any services start, so this
// runs the same commands rc.local used to carry rather than writing them
// out to be run later.
func applyStaticNetwork(iface string, subnet Subnet) error {
	address, cidr, err := subnetAddressCIDR(subnet)
	if err != nil {
		return fmt.Errorf("interface %s: %w", iface, err)
	}

	addrCmd := []string{"ip", "addr", "add", fmt.Sprintf("%s/%d", address, cidr)}
	if net.ParseIP(address).To4() != nil {
		addrCmd = append(addrCmd, "brd", "+")
	}
	addrCmd = append(addrCmd, "dev", iface)

	commands := [][]string{
		{"ip", "link", "set", "dev", iface, "up"},
		addrCmd,
	}
	if subnet.Gateway != "" {
		commands = append(commands, []string{"ip", "route", "add", "default", "via", subnet.Gateway})
	}

	for _, args := range commands {
		cmd := exec.Command(args[0], args[1:]...)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%s: %w: %s", strings.Join(args, " "), err, output)
		}
	}

	return disableService("dhcpcd")
}

// subnetAddressCIDR returns the address and CIDR prefix length to assign
// for subnet. The address may already carry its own "/<prefix>" suffix
// (e.g. "fd8c::1/64"), in which case it's used as-is; otherwise the prefix
// is derived from the dotted-decimal Netmask field.
func subnetAddressCIDR(subnet Subnet) (string, int, error) {
	if address, prefix, ok := strings.Cut(subnet.Address, "/"); ok {
		cidr, err := strconv.Atoi(prefix)
		if err != nil {
			return "", 0, fmt.Errorf("invalid address prefix %q: %w", subnet.Address, err)
		}
		return address, cidr, nil
	}

	if subnet.Netmask == "" {
		return "", 0, fmt.Errorf("address %q has no CIDR prefix and no netmask was given", subnet.Address)
	}

	cidr, err := netmaskToCIDR(subnet.Netmask)
	if err != nil {
		return "", 0, err
	}

	return subnet.Address, cidr, nil
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
	fmt.Fprintln(&sb, userConfigMarker)

	return writeManagedFile(resolvConfPath, sb.String(), 0o644)
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
