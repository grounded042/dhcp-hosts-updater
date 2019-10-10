package main

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/cbednarski/hostess"
)

type externalHostsProvider interface {
	GetHosts() map[string]net.IP
}

func main() {
	address := os.Args[1]
	username := os.Args[2]
	password := os.Args[3]

	p, err := newEdgeOSHostsProvider(address, username, password)
	if err != nil {
		panic(err)
	}

	err = updateHostsFile(p.GetHosts())
	if err != nil {
		panic(err)
	}
}

func updateHostsFile(hosts map[string]net.IP) error {
	hostfile := hostess.NewHostfile()
	err := hostfile.Read()
	if err != nil {
		return err
	}

	errs := hostfile.Parse()
	if len(errs) != 0 {
		return fmt.Errorf("multiple errors parsing hosts file! %v", errs)
	}

	for name, ip := range hosts {
		if name == "" {
			continue
		}

		if removeHostsThatMatchIPAndNotDomain(&hostfile.Hosts, ip, name) {
			continue
		}

		hostfile.Hosts.Add(&hostess.Hostname{
			Domain:  name,
			IP:      ip,
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
