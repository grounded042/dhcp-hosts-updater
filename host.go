package main

import "net"

type Host struct {
	Name string
	IP   net.IP
	MAC  net.HardwareAddr
}
