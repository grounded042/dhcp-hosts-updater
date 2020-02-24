package edgeos

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Provider_ID(t *testing.T) {
	assert.Equal(t, "edgeos", Provider().ID)
}

func Test_Provider_RequiredFlags(t *testing.T) {
	assert.Equal(t, map[string]string{
		"address":  "the address of the edgeos server",
		"username": "the username for the edgeos server",
		"password": "the password for the edgeos server",
	}, Provider().RequiredFlags)
}

func Test_Provider_OptionalFlags(t *testing.T) {
	assert.Equal(t, map[string]string(map[string]string(nil)), Provider().OptionalFlags)
}

func Test_dhcpServerLeaseGroup_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedOutput dhcpServerLeaseGroup
		expectedError  error
	}{
		{
			name:           "empty dhcpServerLease",
			input:          `""`,
			expectedOutput: dhcpServerLeaseGroup{},
		},
		{
			name: "filled dhcpServerLease",
			input: `{
    "192.168.1.2": { "client-hostname": "host-1" },
    "192.168.1.3": { "client-hostname": "host-2" }
}`,
			expectedOutput: dhcpServerLeaseGroup{
				"192.168.1.2": dhcpServerLease{
					ClientHostname: "host-1",
				},
				"192.168.1.3": dhcpServerLease{
					ClientHostname: "host-2",
				},
			},
		},
		{
			name: "errors when unmarshaling failed on a non-empty string",
			input: `{
    "192.168.1.2": "host-1"
}`,
			expectedOutput: dhcpServerLeaseGroup{},
			expectedError:  errors.New("cannot unmarshal dhcpServerLeaseGroup: json: cannot unmarshal string into Go value of type edgeos.dhcpServerLease"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actualOutput := dhcpServerLeaseGroup{}
			err := json.Unmarshal([]byte(tc.input), &actualOutput)
			if tc.expectedError != nil {
				assert.EqualError(t, err, tc.expectedError.Error())
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tc.expectedOutput, actualOutput)
		})
	}
}

func Test_newClient(t *testing.T) {
	expectedUsername := "i-am-username"
	expectedPassword := "i-am-password"
	expectedForm := url.Values{
		"username": []string{expectedUsername},
		"password": []string{expectedPassword},
	}
	httpClient := &http.Client{}
	var actualReq *http.Request
	var actualForm url.Values
	expectedCookie := &http.Cookie{
		Name:    "testingCookie",
		Value:   "cookieValue",
		Expires: time.Now().AddDate(0, 0, 1),
	}

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actualReq = r

		err := r.ParseForm()
		require.NoError(t, err)

		actualForm = r.Form

		http.SetCookie(w, expectedCookie)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	expectedAddress := ts.Listener.Addr().String()

	c, err := newClient(expectedAddress, expectedUsername, expectedPassword, httpClient)
	require.NoError(t, err)

	assert.Equal(t, expectedAddress, c.address)

	assert.Equal(t, expectedAddress, actualReq.Host)
	assert.Equal(t, "/", actualReq.URL.Path)
	assert.Equal(t, expectedForm, actualForm)

	assert.Equal(t, c.httpClient, httpClient)
	cookies := httpClient.Jar.Cookies(&url.URL{Scheme: "https", Host: expectedAddress})
	require.Len(t, cookies, 1)
	assert.Equal(t, expectedCookie.Name, cookies[0].Name)
	assert.Equal(t, expectedCookie.Value, cookies[0].Value)

	require.IsType(t, &http.Transport{}, httpClient.Transport)
	assert.True(t, c.httpClient.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify)
}

func Test_populateDynamicHosts(t *testing.T) {
	tests := []struct {
		name          string
		expectedHosts map[string]net.IP
		handler       func(w http.ResponseWriter, r *http.Request)
		expectedError error
	}{
		{
			name: "populates the hosts",
			expectedHosts: map[string]net.IP{
				"host-1": net.IP{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 255, 255, 192, 168, 1, 25},
				"host-2": net.IP{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 255, 255, 192, 168, 1, 24},
				"host-3": net.IP{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 255, 255, 192, 168, 1, 55},
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				dlr := dhcpLeasesResponse{
					Success: "1",
					Output: dhcpLeasesOutput{
						DHCPServerLeases: map[string]dhcpServerLeaseGroup{
							"Group1": map[string]dhcpServerLease{
								"192.168.1.25": dhcpServerLease{
									ClientHostname: "host-1",
								},
								"192.168.1.24": dhcpServerLease{
									ClientHostname: "host-2",
								},
							},
							"Group2": map[string]dhcpServerLease{
								"192.168.1.55": dhcpServerLease{
									ClientHostname: "host-3",
								},
							},
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				require.NoError(t, json.NewEncoder(w).Encode(dlr))
			},
		},
		{
			name:          "errors if decoding the json errors",
			expectedHosts: nil,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("<"))
			},
			expectedError: errors.New("could not unmarshal response of dynamic hosts: invalid character '<' looking for beginning of value"),
		},
		{
			name: "errors if a non-ok status is returned",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			expectedError: errors.New("request for dynamic hosts returned a non 200 status code \"500\""),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var actualReq *http.Request
			actualHosts := map[string]net.IP{}
			expectedURLPath := "/api/edge/data.json"
			expectedURLQuery := "data=dhcp_leases"

			ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				actualReq = r
				tc.handler(w, r)
			}))
			defer ts.Close()
			expectedAddress := ts.Listener.Addr().String()

			c := client{
				httpClient: &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							InsecureSkipVerify: true,
						},
					},
				},
				address: expectedAddress,
			}

			err := c.populateDynamicHosts(actualHosts)
			if tc.expectedError != nil {
				assert.EqualError(t, err, tc.expectedError.Error())
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, expectedAddress, actualReq.Host)
			if tc.expectedHosts == nil {
				assert.Len(t, actualHosts, 0)
			} else {
				assert.Equal(t, tc.expectedHosts, actualHosts)
			}
			assert.Equal(t, expectedURLPath, actualReq.URL.Path)
			assert.Equal(t, expectedURLQuery, actualReq.URL.RawQuery)
		})
	}
}

func Test_populateStaticHosts(t *testing.T) {
	tests := []struct {
		name          string
		expectedHosts map[string]net.IP
		handler       func(w http.ResponseWriter, r *http.Request)
		expectedError error
	}{
		{
			name: "populates the hosts",
			expectedHosts: map[string]net.IP{
				"host-1": net.IP{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 255, 255, 192, 168, 1, 23},
				"host-2": net.IP{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 255, 255, 192, 168, 2, 2},
				"host-3": net.IP{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 255, 255, 192, 168, 3, 15},
				"host-4": net.IP{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 255, 255, 192, 168, 1, 24},
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				eg := edgeOSGet{
					GET: get{
						Service: edgeOSService{
							DHCPServer: edgeOSDHCPServer{
								SharedNetwork: map[string]edgeOSSharedNetwork{
									"Group1": edgeOSSharedNetwork{
										Subnet: map[string]edgeOSSubnet{
											"192.168.1.0/24": edgeOSSubnet{
												StaticMapping: map[string]edgeOSStaticMapping{
													"host-1": edgeOSStaticMapping{
														IPAddress: "192.168.1.23",
													},
													"host-4": edgeOSStaticMapping{
														IPAddress: "192.168.1.24",
													},
												},
											},
											"192.168.2.0/24": edgeOSSubnet{
												StaticMapping: map[string]edgeOSStaticMapping{
													"host-2": edgeOSStaticMapping{
														IPAddress: "192.168.2.2",
													},
												},
											},
										},
									},
									"Group2": edgeOSSharedNetwork{
										Subnet: map[string]edgeOSSubnet{
											"192.168.3.0/24": edgeOSSubnet{
												StaticMapping: map[string]edgeOSStaticMapping{
													"host-3": edgeOSStaticMapping{
														IPAddress: "192.168.3.15",
													},
												},
											},
										},
									},
								},
							},
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				require.NoError(t, json.NewEncoder(w).Encode(eg))
			},
		},
		{
			name:          "errors if decoding the json errors",
			expectedHosts: nil,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("<"))
			},
			expectedError: errors.New("could not unmarshal response of static hosts: invalid character '<' looking for beginning of value"),
		},
		{
			name: "errors if a non-ok status is returned",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			expectedError: errors.New("request for static hosts returned a non 200 status code \"500\""),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var actualReq *http.Request
			actualHosts := map[string]net.IP{}
			expectedURLPath := "/api/edge/get.json"

			ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				actualReq = r
				tc.handler(w, r)
			}))
			defer ts.Close()
			expectedAddress := ts.Listener.Addr().String()

			c := client{
				httpClient: &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: &tls.Config{
							InsecureSkipVerify: true,
						},
					},
				},
				address: expectedAddress,
			}

			err := c.populateStaticHosts(actualHosts)
			if tc.expectedError != nil {
				assert.EqualError(t, err, tc.expectedError.Error())
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, expectedAddress, actualReq.Host)
			if tc.expectedHosts == nil {
				assert.Len(t, actualHosts, 0)
			} else {
				assert.Equal(t, tc.expectedHosts, actualHosts)
			}
			assert.Equal(t, expectedURLPath, actualReq.URL.Path)
		})
	}
}
