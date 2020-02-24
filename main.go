package main

import (
	"os"

	"github.com/grounded042/dhcp-hosts-updater/internal/edgeos"
	"github.com/grounded042/dhcp-hosts-updater/pkg/host"
)

// dhcp-hosts-updater --server edgeos --address <addr> --username <username> --password <password>

func main() {
	updater := host.NewUpdater().
		WithServer(edgeos.Provider())

	server := os.Args[1]
	flags := map[string]string{
		"address":  os.Args[2],
		"username": os.Args[3],
		"password": os.Args[4],
	}

	if err := updater.Update(server, flags); err != nil {
		panic(err)
	}
}
