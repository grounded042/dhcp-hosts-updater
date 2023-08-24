package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
)

type sitesResponse struct {
	Data []struct {
		Name string `json:"name"`
	} `json:"data"`
}

type clientResponse []struct {
	DisplayName string `json:"display_name"`
	IP          string `json:"ip"`
	Hostname    string `json:"hostname"`
	MAC         string `json:"mac"`
}

type udmProHostsProvider struct {
	client  *http.Client
	address string
	site    string
}

func (u *udmProHostsProvider) GetHosts() ([]Host, error) {
	resp, err := u.client.Get(fmt.Sprintf("https://%s/proxy/network/v2/api/site/%s/clients/active", u.address, u.site))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	decodedResp := clientResponse{}

	err = json.NewDecoder(resp.Body).Decode(&decodedResp)
	if err != nil {
		return nil, err
	}

	toReturn := []Host{}
	for _, device := range decodedResp {
		mac, err := net.ParseMAC(device.MAC)
		if err != nil {
			return nil, err
		}
		if len(device.Hostname) == 0 {
			device.Hostname = device.DisplayName
		}
		toReturn = append(toReturn, Host{
			Name: device.Hostname,
			IP:   net.ParseIP(device.IP),
			MAC:  mac,
		})
	}

	return toReturn, nil
}

func newUDMProHostsProvider(address, username, password string) (externalHostsProvider, error) {
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

	values := map[string]string{"username": username, "password": password}

	jsonValue, _ := json.Marshal(values)

	_, err = client.Post(fmt.Sprintf("https://%s/api/auth/login", address), "application/json", bytes.NewBuffer(jsonValue))
	if err != nil {
		return nil, err
	}

	resp, err := client.Get(fmt.Sprintf("https://%s/proxy/network/api/self/sites", address))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	decodedResp := sitesResponse{}

	err = json.NewDecoder(resp.Body).Decode(&decodedResp)
	if err != nil {
		return nil, err
	}

	return &udmProHostsProvider{
		client:  client,
		address: address,
		site:    decodedResp.Data[0].Name,
	}, nil
}
