package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
)

type dhcpLeasesResponse struct {
	Success string           `json:"success"`
	Output  dhcpLeasesOutput `json:"output"`
}

type dhcpLeasesOutput struct {
	DHCPServerLeases map[string]map[string]dhcpServerLease `json:"dhcp-server-leases"`
}

type dhcpServerLease struct {
	Expiration     string `json:"expiration"`
	Pool           string `json:"pool"`
	Mac            string `json:"mac"`
	ClientHostname string `json:"client-hostname"`
}

type edgeOSHostsProvider struct {
	client  *http.Client
	address string
}

func (e *edgeOSHostsProvider) GetHosts() map[string]net.IP {
	resp, err := e.client.Get(fmt.Sprintf("https://%s/api/edge/data.json?data=dhcp_leases", e.address))
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	decodedResp := dhcpLeasesResponse{}

	err = json.NewDecoder(resp.Body).Decode(&decodedResp)
	if err != nil {
		panic(err)
	}

	toReturn := e.getStaticHosts()
	for _, value := range decodedResp.Output.DHCPServerLeases {
		for ip, details := range value {
			toReturn[details.ClientHostname] = net.ParseIP(ip)
		}
	}

	return toReturn
}

func (e *edgeOSHostsProvider) getStaticHosts() map[string]net.IP {
	resp, err := e.client.Get(fmt.Sprintf("https://%s/api/edge/get.json", e.address))
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	decodedResp := edgeOSGet{}

	err = json.NewDecoder(resp.Body).Decode(&decodedResp)
	if err != nil {
		panic(err)
	}

	toReturn := map[string]net.IP{}
	for _, sharedNetwork := range decodedResp.GET.Service.DHCPServer.SharedNetwork {
		for _, subnet := range sharedNetwork.Subnet {
			for name, staticMapping := range subnet.StaticMapping {
				toReturn[name] = net.ParseIP(staticMapping.IPAddress)
			}
		}
	}

	return toReturn
}

func newEdgeOSHostsProvider(address, username, password string) (externalHostsProvider, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Jar: jar,
	}

	v := url.Values{
		"username": []string{username},
		"password": []string{password},
	}

	res, err := client.PostForm(fmt.Sprintf("https://%s/", address), v)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	return &edgeOSHostsProvider{
		client:  client,
		address: address,
	}, nil
}

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
