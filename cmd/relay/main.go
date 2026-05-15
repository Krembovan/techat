package main

import (
	"flag"
	"fmt"
	"os"

	"techat/internal/network"
)

func main() {
	var addr string
	flag.StringVar(&addr, "addr", "0.0.0.0:7777", "Relay server listen address")
	flag.Parse()

	relay := network.NewRelay(addr)
	fmt.Fprintf(os.Stderr, "TeChat Relay Server starting on %s\n", addr)
	fmt.Fprintf(os.Stderr, "Clients connect with: techat --relay %s\n", addr)

	if err := relay.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Relay server error: %v\n", err)
		os.Exit(1)
	}
}
