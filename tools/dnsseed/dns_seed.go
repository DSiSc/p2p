package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"github.com/DSiSc/craft/log"
	"github.com/DSiSc/craft/types"
	"github.com/DSiSc/p2p"
	"github.com/DSiSc/p2p/common"
	p2pconf "github.com/DSiSc/p2p/config"
	"github.com/DSiSc/p2p/message"
	"github.com/DSiSc/p2p/tools"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

// process system signal
func sysSignalProcess() {
	c := make(chan os.Signal)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	<-c
	os.Exit(0)
}

// init system log config
func initLog(conf NodeConfig) {
	var logPath string
	if conf.Logger.Appenders[FileLogAppender].Enabled {
		// initialize logfile
		logPath = conf.Logger.Appenders[FileLogAppender].LogPath
		EnsureFolderExist(logPath[0:strings.LastIndex(logPath, "/")])
		logfile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0644)
		if err != nil {
			panic(err)
		}
		conf.Logger.Appenders[FileLogAppender].Output = logfile
	}

	log.SetGlobalConfig(&conf.Logger)
}

func main() {
	node := NewNodeConfig()
	initLog(node)

	// init p2p config
	conf := &p2pconf.P2PConfig{
		AddrBookFilePath: node.AddrBookFilePath,
		ListenAddress:    node.ListenAddress,
		MaxConnOutBound:  node.MaxConnOutBound,
		MaxConnInBound:   node.MaxConnInBound,
		SeedMode:         true,
		WorkerPoolSize:   1000,
	}

	// create p2p
	p2p, err := p2p.NewP2P(conf, tools.NewP2PTestEventCenter())
	if err != nil {
		fmt.Printf("failed to create p2p node, as: %v", err)
		os.Exit(1)
	}

	// start p2p
	err = p2p.Start()
	if err != nil {
		fmt.Printf("failed to start p2p node, as: %v", err)
		os.Exit(1)
	}
	// catch system exit signal
	sysSignalProcess()
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
