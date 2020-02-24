package edgeos

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"

	"golang.org/x/net/publicsuffix"

	"github.com/grounded042/dhcp-hosts-updater/pkg/host"
)

const (
	addressFlag  = "address"
	usernameFlag = "username"
	passwordFlag = "password"
)

// Provider builds and returns an EdgeOS provider.
func Provider() *host.ServerProvider {
	return &host.ServerProvider{
		ID: "edgeos",
		RequiredFlags: map[string]string{
			addressFlag:  "the address of the edgeos server",
			usernameFlag: "the username for the edgeos server",
			passwordFlag: "the password for the edgeos server",
		},
		GetHostsFn: func(flags map[string]string) (map[string]net.IP, error) {
			c, err := newClient(flags[addressFlag], flags[usernameFlag], flags[passwordFlag], &http.Client{})
			if err != nil {
				return nil, fmt.Errorf("could not build edgeos client: %w", err)
			}

			hosts := map[string]net.IP{}
			if err := c.populateDynamicHosts(hosts); err != nil {
				return nil, err
			}
			if err := c.populateStaticHosts(hosts); err != nil {
				return nil, err
			}

			return hosts, nil

		},
	}
}

// dhcp response from edgeos

type dhcpLeasesResponse struct {
	Success string           `json:"success"`
	Output  dhcpLeasesOutput `json:"output"`
}

type dhcpLeasesOutput struct {
	DHCPServerLeases map[string]dhcpServerLeaseGroup `json:"dhcp-server-leases"`
}

type dhcpServerLeaseGroup map[string]dhcpServerLease

type dhcpServerLease struct {
	ClientHostname string `json:"client-hostname"`
}

func (dslg *dhcpServerLeaseGroup) UnmarshalJSON(b []byte) error {
	raw := map[string]dhcpServerLease{}
	if err := json.Unmarshal(b, &raw); err != nil {
		if err.Error() == "json: cannot unmarshal string into Go value of type map[string]edgeos.dhcpServerLease" {
			return nil
		}
		return fmt.Errorf("cannot unmarshal dhcpServerLeaseGroup: %w", err)
	}

	*dslg = raw

	return nil
}

// static resposne from edgeos

type edgeOSGet struct {
	GET get `json:"GET"`
}
type get struct {
	Service edgeOSService `json:"service"`
}

type edgeOSService struct {
	DHCPServer edgeOSDHCPServer `json:"dhcp-server"`
}

type edgeOSDHCPServer struct {
	SharedNetwork map[string]edgeOSSharedNetwork `json:"shared-network-name"`
}

type edgeOSSharedNetwork struct {
	Subnet map[string]edgeOSSubnet `json:"subnet"`
}

type edgeOSSubnet struct {
	StaticMapping map[string]edgeOSStaticMapping `json:"static-mapping"`
}

type edgeOSStaticMapping struct {
	IPAddress string `json:"ip-address"`
}

// client

type client struct {
	httpClient *http.Client
	address    string
}

func newClient(address, username, password string, httpClient *http.Client) (*client, error) {
	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return nil, err
	}

	httpClient.Transport = &http.Transport{
		// TODO: make this optional
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	httpClient.Jar = jar

	v := url.Values{
		"username": []string{username},
		"password": []string{password},
	}

	if _, err = httpClient.PostForm(fmt.Sprintf("https://%s/", address), v); err != nil {
		return nil, err
	}

	return &client{
		httpClient: httpClient,
		address:    address,
	}, nil
}

func (c *client) populateDynamicHosts(hosts map[string]net.IP) error {
	resp, err := c.httpClient.Get(fmt.Sprintf("https://%s/api/edge/data.json?data=dhcp_leases", c.address))
	if err != nil {
		return fmt.Errorf("could get dynamic hosts: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("request for dynamic hosts returned a non 200 status code \"%d\"", resp.StatusCode)
	}

	decodedResp := dhcpLeasesResponse{}
	err = json.NewDecoder(resp.Body).Decode(&decodedResp)
	if err != nil {
		return fmt.Errorf("could not unmarshal response of dynamic hosts: %w", err)
	}

	for _, group := range decodedResp.Output.DHCPServerLeases {
		for ip, details := range group {
			hosts[details.ClientHostname] = net.ParseIP(ip)
		}
	}

	return nil
}

func (c *client) populateStaticHosts(hosts map[string]net.IP) error {
	resp, err := c.httpClient.Get(fmt.Sprintf("https://%s/api/edge/get.json", c.address))
	if err != nil {
		return fmt.Errorf("could get static hosts: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("request for static hosts returned a non 200 status code \"%d\"", resp.StatusCode)
	}

	decodedResp := edgeOSGet{}
	err = json.NewDecoder(resp.Body).Decode(&decodedResp)
	if err != nil {
		return fmt.Errorf("could not unmarshal response of static hosts: %w", err)
	}

	for _, sharedNetwork := range decodedResp.GET.Service.DHCPServer.SharedNetwork {
		for _, subnet := range sharedNetwork.Subnet {
			for name, staticMapping := range subnet.StaticMapping {
				hosts[name] = net.ParseIP(staticMapping.IPAddress)
			}
		}
	}

	return nil
}
