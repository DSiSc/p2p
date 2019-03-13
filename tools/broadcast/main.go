package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"flag"
	"fmt"
	"github.com/DSiSc/blockchain"
	"github.com/DSiSc/blockchain/config"
	"github.com/DSiSc/craft/types"
	"github.com/DSiSc/p2p"
	"github.com/DSiSc/p2p/common"
	p2pconf "github.com/DSiSc/p2p/config"
	"github.com/DSiSc/p2p/message"
	"github.com/DSiSc/p2p/tools"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

func sysSignalProcess() {
	c := make(chan os.Signal)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	<-c
	os.Exit(0)
}

func main() {
	chainConf := config.BlockChainConfig{
		PluginName: blockchain.PLUGIN_MEMDB,
	}
	blockchain.InitBlockChain(chainConf, &tools.P2PTestEventCenter{})
	var addrBookPath, listenAddress, persistentPeers, localAddrStr, displayServer, dnsSeeds string
	var maxConnOutBound, maxConnInBound int
	var traceMaster, disableDNSSeed, seedMode bool
	flagSet := flag.NewFlagSet("broadcast", flag.ExitOnError)
	flagSet.StringVar(&addrBookPath, "path", "./address_book.json", "Address book file path")
	flagSet.StringVar(&listenAddress, "listen", "tcp://0.0.0.0:8888", "Listen address")
	flagSet.StringVar(&persistentPeers, "peers", "", "Persistent peers")
	flagSet.IntVar(&maxConnOutBound, "out", 4, "Max num of outbound peer")
	flagSet.IntVar(&maxConnInBound, "in", 8, "Max num of out inbound peer")
	flagSet.StringVar(&localAddrStr, "local_addr", "", "local address to identify this peer")
	flagSet.StringVar(&displayServer, "display_server", "localhost:8080", "Statistics server address")
	flagSet.BoolVar(&traceMaster, "master", false, "Master peer")
	flagSet.BoolVar(&disableDNSSeed, "disable_dns", true, "disable DNS seeding for peers(default true)")
	flagSet.BoolVar(&seedMode, "seed_mode", false, "whether run as dns seed(default false)")
	flagSet.StringVar(&dnsSeeds, "dns_seeds", "", "list of DNS seeds ")

	flagSet.Usage = func() {
		fmt.Println(`Justitia blockchain p2p test tool.

Usage:
	broadcast [-path ./address_book.json] [-display_server localhost:8080] [-local_addr 192.168.1.101] [-listen tcp://0.0.0.0:8080] [-peers tcp://192.168.1.100:1080] [-out 4] [-in 8]

Examples:
	broadcast -local_addr 192.168.1.101 -peers tcp://192.168.1.100:1080`)
		fmt.Println("Flags:")
		flagSet.PrintDefaults()
	}
	flagSet.Parse(os.Args[1:])

	// init p2p config
	conf := &p2pconf.P2PConfig{
		AddrBookFilePath: addrBookPath,
		ListenAddress:    listenAddress,
		PersistentPeers:  persistentPeers,
		MaxConnOutBound:  maxConnOutBound,
		MaxConnInBound:   maxConnInBound,
		DebugServer:      displayServer,
		DebugP2P:         true,
		DebugAddr:        localAddrStr,
		SeedMode:         seedMode,
		DisableDNSSeed:   disableDNSSeed,
		DNSSeeds:         dnsSeeds,
		Service:          p2pconf.SFNodeBroadCastTest,
	}

	// listen address
	listenAddr, err := common.ParseNetAddress(listenAddress)
	if err != nil {
		fmt.Printf("invalid listen address, as: %v", err)
		os.Exit(1)
	}
	localAddr, err := common.ParseNetAddress(localAddrStr + ":" + strconv.Itoa(int(listenAddr.Port)))
	if err != nil {
		fmt.Printf("invalid local_addr, as: %v", err)
		os.Exit(1)
	}

	// create p2p
	p2p, err := p2p.NewP2P(conf, tools.NewP2PTestEventCenter())
	if err != nil {
		fmt.Printf("failed to create p2p, as: %v", err)
		os.Exit(1)
	}

	// start p2p
	err = p2p.Start()
	if err != nil {
		fmt.Printf("failed to start p2p, as: %v", err)
		os.Exit(1)
	}

	// start trace handler
	taceHandler := NewTraceMsgHandler(localAddr, p2p)
	taceHandler.Start()

	// catch system exit signal
	go sysSignalProcess()

	// init sleep, Wait for the connection to be established successfully
	time.Sleep(120 * time.Second)
	// send trace message periodically
	timer := time.NewTicker(30 * time.Second)
	for {
		if traceMaster {
			tmsg := newTraceMsg(localAddr)
			p2p.BroadCast(tmsg)
		}
		<-timer.C
	}
}

// create a random trace message
func newTraceMsg(localAddr *common.NetAddress) *message.TraceMsg {
	id := randomHash()
	fmt.Printf("%x", id)
	return &message.TraceMsg{
		ID: id,
	}
}

// create a random hash
func randomHash() (hash types.Hash) {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, rand.Int63())
	hasher := sha256.New()
	hasher.Write(buf.Bytes())
	copy(hash[:], hasher.Sum(nil))
	return
}

// TraceMsgHandler is the trace message handler
type TraceMsgHandler struct {
	localAddr *common.NetAddress
	p2p       p2p.P2PAPI
	quitChan  chan interface{}
}

// NewTraceMsgHandler create a new trace message handler
func NewTraceMsgHandler(localAddr *common.NetAddress, p2p p2p.P2PAPI) *TraceMsgHandler {
	return &TraceMsgHandler{
		localAddr: localAddr,
		p2p:       p2p,
		quitChan:  make(chan interface{}),
	}
}

// Start start handler
func (this *TraceMsgHandler) Start() {
	go this.recvHandler()
}

// Stop stop handler
func (this *TraceMsgHandler) Stop() {
	close(this.quitChan)
}

// message receive routine
func (this *TraceMsgHandler) recvHandler() {
	msgChan := this.p2p.MessageChan()
	for {
		select {
		case msg := <-msgChan:
			switch msg.Payload.MsgType() {
			case message.TRACE_TYPE:
				tmsg := msg.Payload.(*message.TraceMsg)
				this.p2p.BroadCast(tmsg)
			}
		case <-this.quitChan:
			return
		}
	}

}
