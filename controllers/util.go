// Copyright 2023 The casbin Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controllers

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/casvisor/casvisor/util"
)

type Response struct {
	Status string      `json:"status"`
	Msg    string      `json:"msg"`
	Data   interface{} `json:"data"`
	Data2  interface{} `json:"data2"`
}

func (c *ApiController) ResponseOk(data ...interface{}) {
	resp := Response{Status: "ok"}
	switch len(data) {
	case 2:
		resp.Data2 = data[1]
		fallthrough
	case 1:
		resp.Data = data[0]
	}
	c.Data["json"] = resp
	c.ServeJSON()
}

func (c *ApiController) ResponseError(error string, data ...interface{}) {
	resp := Response{Status: "error", Msg: error}
	switch len(data) {
	case 2:
		resp.Data2 = data[1]
		fallthrough
	case 1:
		resp.Data = data[0]
	}
	c.Data["json"] = resp
	c.ServeJSON()
}

func (c *ApiController) RequireSignedIn() bool {
	if c.GetSessionUser() == nil {
		c.ResponseError("please sign in first")
		return true
	}

	return false
}

func (c *ApiController) getClientIp() string {
	res := strings.Replace(util.GetIPFromRequest(c.Ctx.Request), ": ", "", -1)
	return res
}

func (c *ApiController) getUserAgent() string {
	res := c.Ctx.Request.UserAgent()
	return res
}

func isIpAddress(host string) bool {
	// Attempt to split the host and port, ignoring the error
	hostWithoutPort, _, err := net.SplitHostPort(host)
	if err != nil {
		// If an error occurs, it might be because there's no port
		// In that case, use the original host string
		hostWithoutPort = host
	}

	// Attempt to parse the host as an IP address (both IPv4 and IPv6)
	ip := net.ParseIP(hostWithoutPort)
	// if host is not nil is an IP address else is not an IP address
	return ip != nil
}

func getOriginFromHost(host string) string {
	protocol := "https://"
	if !strings.Contains(host, ".") {
		// "localhost:14000"
		protocol = "http://"
	} else if isIpAddress(host) {
		// "192.168.0.10"
		protocol = "http://"
	}

	return fmt.Sprintf("%s%s", protocol, host)
}

func getLocalNetworkInfo() ([]*net.IPNet, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var addrlist []*net.IPNet
	for _, iface := range interfaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagLoopback == 0 {
				switch v := addr.(type) {
				case *net.IPNet:
					if !v.IP.IsLoopback() && v.IP.To4() != nil {
						addrlist = append(addrlist, v)
					}
				}
			}
		}
	}
	if len(addrlist) == 0 {
		return nil, fmt.Errorf("no suitable network interface found")
	}
	return addrlist, nil
}

func scanIPsAndPortsInNetwork(network *net.IPNet) []string {
	var wg sync.WaitGroup
	const maxWorkers = 1000
	semaphore := make(chan bool, maxWorkers)
	resultChan := make(chan string)

	for ip := network.IP.Mask(network.Mask); network.Contains(ip); incrementIP(ip) {
		wg.Add(1)
		semaphore <- true
		ip0 := append(net.IP(nil), ip...)
		go func(ip net.IP) {
			defer func() {
				<-semaphore
				wg.Done()
			}()
			targetPort := scanIPAndPort(ip.String())
			if targetPort != "" {
				resultChan <- net.JoinHostPort(ip.String(), targetPort)
			}
		}(ip0)
	}
	go func() {
		wg.Wait()
		close(resultChan)
	}()
	var results []string
	for result := range resultChan {
		results = append(results, result)
	}
	return results
}

func scanIPAndPort(ip string) string {
	targetPorts := []string{"22", "3389"}
	for _, targetPort := range targetPorts {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, targetPort), 2*time.Second)
		if err == nil {
			conn.Close()
			return targetPort
		}
	}
	return ""
}

func incrementIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}
