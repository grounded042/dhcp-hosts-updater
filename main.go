package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"

	"github.com/cbednarski/hostess"
	"github.com/mitchellh/cli"
)

type externalHostsProvider interface {
	GetHosts() ([]Host, error)
}

type flagStringSlice []string

var _ flag.Value = (*flagStringSlice)(nil)

func (v *flagStringSlice) String() string {
	return ""
}
func (v *flagStringSlice) Set(raw string) error {
	*v = append(*v, raw)

	return nil
}

func main() {
	c := cli.NewCLI("dhcp-hosts-updater", "0.0.1")
	c.Args = os.Args[1:]
	c.Commands = map[string]cli.CommandFactory{
		"foo": fooCommandFactory,
		"bar": barCommandFactory,
	}

	exitStatus, err := c.Run()
	if err != nil {
		log.Println(err)
	}

	os.Exit(exitStatus)

	addressFlag := flag.String("address", "", "The address of the DHCP server")
	usernameFlag := flag.String("username", "", "The username for the DHCP server")
	passwordFlag := flag.String("password", "", "The password for the DHCP server")

	macOverride := &flagStringSlice{}
	flag.Var(macOverride, "mac-override", "MAC address overrides in mac=overridden-hostname format. Can be specific multiple times.")

	flag.Parse()

	macOverrides, err := parseMacOverrides(*macOverride)
	if err != nil {
		panic(err)
	}

	// p, err := newEdgeOSHostsProvider(address, username, password)
	p, err := newUDMProHostsProvider(*addressFlag, *usernameFlag, *passwordFlag)
	if err != nil {
		panic(err)
	}

	hosts, err := p.GetHosts()
	if err != nil {
		panic(err)
	}

	for i, host := range hosts {
		if strings.Contains(host.Name, " ") {
			hosts[i].Name = strings.ReplaceAll(host.Name, " ", "-")
		}

		if override, exists := macOverrides[strings.ToLower(host.MAC.String())]; exists {
			hosts[i].Name = override
		}

		fmt.Println(host.Name, host.MAC)
	}

	err = updateHostsFile(hosts)

	if err != nil {
		panic(err)
	}
}

func parseMacOverrides(overrides []string) (map[string]string, error) {
	toReturn := map[string]string{}

	for _, override := range overrides {
		parts := strings.Split(override, "=")
		if len(parts) != 2 {
			return nil, fmt.Errorf("MAC override %q was not properly formatted as mac=overridden-hostname", override)
		}
		toReturn[strings.ToLower(parts[0])] = parts[1]
	}

	return toReturn, nil
}

func updateHostsFile(hosts []Host) error {
	hostfile := hostess.NewHostfile()
	err := hostfile.Read()
	if err != nil {
		return err
	}

	errs := hostfile.Parse()
	if len(errs) != 0 {
		return fmt.Errorf("multiple errors parsing hosts file! %v", errs)
	}

	for _, host := range hosts {
		if host.Name == "" {
			continue
		}

		if removeHostsThatMatchIPAndNotDomain(&hostfile.Hosts, host.IP, host.Name) {
			continue
		}

		hostfile.Hosts.Add(&hostess.Hostname{
			Domain:  host.Name,
			IP:      host.IP,
			Enabled: true,
		})
	}

	return hostfile.Save()
}

// removeHostsThatMatchIPAndNotDomain removes any hosts from the host list that
// have the passed in IP but do not have the domain associated with the IP. It
// returns true if there is an entry for the ip that has the matching domain
func removeHostsThatMatchIPAndNotDomain(hosts *hostess.Hostlist, ip net.IP, domain string) bool {
	entryForIPHasDomain := false

	if hosts.ContainsIP(ip) {
		for _, matchingEntry := range hosts.FilterByIP(ip) {
			if strings.ToLower(matchingEntry.Domain) == strings.ToLower(domain) {
				if !matchingEntry.Enabled {
					hosts.Enable(domain)
				}
				entryForIPHasDomain = true
			} else {
				hosts.Remove(hosts.IndexOf(matchingEntry))
			}
		}
	}

	return entryForIPHasDomain
}
