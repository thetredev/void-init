package main

import (
	"os"
	"testing"
)

func TestParseNetworkConfigSample(t *testing.T) {
	data, err := os.ReadFile("testfiles/network-config")
	if err != nil {
		t.Fatal(err)
	}

	nc, err := ParseNetworkConfig(data)
	if err != nil {
		t.Fatal(err)
	}

	if nc.Version != 1 {
		t.Errorf("Version = %d, want 1", nc.Version)
	}
	if len(nc.Config) != 3 {
		t.Fatalf("len(Config) = %d, want 3", len(nc.Config))
	}

	eth0, eth1, nameserver := nc.Config[0], nc.Config[1], nc.Config[2]

	if eth0.Type != "physical" || eth0.Name != "eth0" || eth0.MacAddress != "bc:24:11:b7:aa:aa" {
		t.Errorf("eth0 = %+v", eth0)
	}
	if len(eth0.Subnets) != 1 || eth0.Subnets[0].Type != "ipv6_slaac" {
		t.Errorf("eth0.Subnets = %+v", eth0.Subnets)
	}

	if eth1.Type != "physical" || eth1.Name != "eth1" {
		t.Errorf("eth1 = %+v", eth1)
	}
	if len(eth1.Subnets) != 1 {
		t.Fatalf("len(eth1.Subnets) = %d, want 1", len(eth1.Subnets))
	}
	if got, want := eth1.Subnets[0].Type, "static6"; got != want {
		t.Errorf("eth1.Subnets[0].Type = %q, want %q", got, want)
	}
	if got, want := eth1.Subnets[0].Address, "fd8c:314f:f123::900/64"; got != want {
		t.Errorf("eth1.Subnets[0].Address = %q, want %q", got, want)
	}

	if nameserver.Type != "nameserver" {
		t.Errorf("nameserver.Type = %q, want %q", nameserver.Type, "nameserver")
	}
	if len(nameserver.Address) != 1 || nameserver.Address[0] != "fd8c:314f:f123::1" {
		t.Errorf("nameserver.Address = %v", nameserver.Address)
	}
	if len(nameserver.Search) != 1 || nameserver.Search[0] != "my.domain" {
		t.Errorf("nameserver.Search = %v", nameserver.Search)
	}
}

func TestSubnetAddressCIDR(t *testing.T) {
	cases := []struct {
		name        string
		subnet      Subnet
		wantAddress string
		wantCIDR    int
		wantErr     bool
	}{
		{
			name:        "address with embedded prefix",
			subnet:      Subnet{Address: "fd8c:314f:f123::5:9000/64"},
			wantAddress: "fd8c:314f:f123::5:9000",
			wantCIDR:    64,
		},
		{
			name:        "address with dotted netmask",
			subnet:      Subnet{Address: "192.168.1.10", Netmask: "255.255.255.0"},
			wantAddress: "192.168.1.10",
			wantCIDR:    24,
		},
		{
			name:    "no prefix and no netmask",
			subnet:  Subnet{Address: "192.168.1.10"},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			address, cidr, err := subnetAddressCIDR(tc.subnet)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if address != tc.wantAddress {
				t.Errorf("address = %q, want %q", address, tc.wantAddress)
			}
			if cidr != tc.wantCIDR {
				t.Errorf("cidr = %d, want %d", cidr, tc.wantCIDR)
			}
		})
	}
}
