package host

import (
	"fmt"
	"net"
	"strings"

	"github.com/cbednarski/hostess"
)

// ServerProvider holds all the information needed by the Updater to gather
// host information from a host server.
type ServerProvider struct {
	// ID identifies this server and is used to choose which server provider to
	// use on updating.
	ID string
	// RequiredFlags holds the required flags and their descriptions for the
	// updater to use for validating flags for the provider and for providing
	// help to the user.
	RequiredFlags map[string]string
	// RequiredFlags holds the optional flags and their descriptions for the
	// updater to use provide help to the user.
	OptionalFlags map[string]string
	// GetHostsFn is called to get the hosts with which to update the hosts
	// file with. Flags are guaranteed to be at least required flags. If you
	// have optional flags they should be checked to be in the map before using
	// them.
	GetHostsFn func(flags map[string]string) (map[string]net.IP, error)
}

// Updater is used to updated the hosts file based using a provided
// ServerProvider.
type Updater struct {
	servers map[string]ServerProvider
}

// NewUpdater handlesbuilds a new Updater.
func NewUpdater() *Updater {
	return &Updater{
		servers: map[string]ServerProvider{},
	}
}

// WithServer adds the provided ServerProvider to the Updater and returns the
// Updater for easier chaining.
func (u *Updater) WithServer(server *ServerProvider) *Updater {
	u.servers[server.ID] = *server
	return u
}

// Update updates the host file using the ServerProvider with the ID of the
// passed in server. It will pass the flags to the ServerProviders GetHostsFn
// after validating the required flags are there.
func (u *Updater) Update(server string, flags map[string]string) error {
	hosts, err := u.servers[server].GetHostsFn(flags)
	if err != nil {
		return fmt.Errorf("could not update.... %w", err)
	}
	return updateHostsFile(hosts)
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

	fmt.Println("hostfile.Hosts")
	hostsJSON, err := hostfile.Hosts.Dump()
	if err != nil {
		panic(err)
	}
	fmt.Println(string(hostsJSON))

	return nil
	// return hostfile.Save()
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
