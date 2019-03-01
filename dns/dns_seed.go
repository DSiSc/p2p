package main

import (
	"flag"
	"fmt"
	"github.com/DSiSc/craft/log"
	"github.com/DSiSc/p2p"
	"github.com/DSiSc/p2p/config"
	"os"
	"os/signal"
	"syscall"
)

func sysSignalProcess(p *p2p.P2P) {
	c := make(chan os.Signal)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	<-c
	fmt.Println("Stop DNS Seed")
	p.Stop()
	os.Exit(0)
}

// Run as a dns seed.
// DnsSeed defines the node that are used as a public proxy to discover peers.
func main() {
	var addrBookPath, listenAddress, persistentPeers string
	var maxConnOutBound, maxConnInBound int
	flagSet := flag.NewFlagSet("dns-seed", flag.ExitOnError)
	flagSet.StringVar(&addrBookPath, "path", "./address_book.json", "Address book file path")
	flagSet.StringVar(&listenAddress, "listen", "tcp://0.0.0.0:8888", "Listen address")
	flagSet.IntVar(&maxConnOutBound, "out", 4, "Maximum number of connected outbound peers")
	flagSet.IntVar(&maxConnInBound, "in", 8, "Maximum number of connected inbound peers")
	flagSet.Usage = func() {
		fmt.Println(`Justitia blockchain dns seed.

Usage:
	dns-seed [-path ./address_book.json] [-listen tcp://0.0.0.0:8080]

Examples:
	dns-seed -path ./address_book.json -listen tcp://0.0.0.0:8080`)
		fmt.Println("Flags:")
		flagSet.PrintDefaults()
	}
	flagSet.Parse(os.Args[1:])

	// init p2p config
	conf := &config.P2PConfig{
		AddrBookFilePath: addrBookPath,
		ListenAddress:    listenAddress,
		PersistentPeers:  persistentPeers,
		MaxConnOutBound:  maxConnOutBound,
		MaxConnInBound:   maxConnInBound,
		SeedMode:         true,
	}
	dnsSeed, err := p2p.NewP2P(conf, nil)
	if err != nil {
		log.Error("failed to new p2p server, as: %v", err)
	}
	dnsSeed.Start()
	// catch system exit signal
	sysSignalProcess(dnsSeed)
}
