package p2p

import (
	"errors"
	"fmt"
	"github.com/DSiSc/craft/log"
	"github.com/DSiSc/craft/types"
	"github.com/DSiSc/p2p/common"
	"github.com/DSiSc/p2p/config"
	"github.com/DSiSc/p2p/message"
	"github.com/DSiSc/p2p/nat"
	"github.com/DSiSc/p2p/version"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	persistentPeerRetryInterval = time.Minute
	stallTickInterval           = 15 * time.Second
	stallResponseTimeout        = 60 * time.Second
	heartBeatInterval           = 10 * time.Second
)

// PeerFilter used To filter the peer satisfy the request
type PeerFilter func(peerState uint64) bool

// P2P is p2p service implementation.
type P2P struct {
	PeerCom
	config        *config.P2PConfig
	listener      net.Listener // net listener
	internalChan  chan *InternalMsg
	msgChan       chan *InternalMsg
	stallChan     chan *InternalMsg
	quitChan      chan struct{}
	isRunning     int32
	addrManager   *AddressManager
	pendingPeers  sync.Map
	outbountPeers sync.Map
	inboundPeers  sync.Map
	center        types.EventCenter
	lock          sync.RWMutex
	debugHandler  *DebugHandler
}

// NewP2P create a p2p service instance
func NewP2P(config *config.P2PConfig, center types.EventCenter) (*P2P, error) {
	netAddr, err := common.ParseNetAddress(config.ListenAddress)
	if err != nil {
		log.Error("invalid listen address")
		return nil, err
	}
	addrManger := NewAddressManager(config.AddrBookFilePath)
	return &P2P{
		PeerCom: PeerCom{
			version: version.Version,
			addr:    netAddr,
			service: config.Service,
		},
		config:       config,
		addrManager:  addrManger,
		msgChan:      make(chan *InternalMsg),
		internalChan: make(chan *InternalMsg),
		stallChan:    make(chan *InternalMsg),
		quitChan:     make(chan struct{}),
		isRunning:    0,
		center:       center,
	}, nil
}

// Start start p2p service
func (service *P2P) Start() error {
	service.lock.Lock()
	defer service.lock.Unlock()
	log.Info("Begin starting p2p")
	if service.isRunning != 0 {
		log.Error("P2P already started")
		return fmt.Errorf("P2P already started")
	}

	service.addrManager.Start()

	err := service.addrManager.AddLocalAddress(service.addr.Port)
	if err != nil {
		log.Error("failed To add local address To address manager, as %v", err)
		return err
	}

	listener, err := net.Listen(service.addr.Protocol, service.addr.IP+":"+strconv.Itoa(int(service.addr.Port)))
	if err != nil {
		log.Error("failed To create listener with address: %s, as: %v", service.addr.ToString(), err)
		return err
	}
	service.listener = listener
	go service.startListen(listener) // listen To accept new connection
	if "" != service.config.NAT {
		go service.addPortMapping(int(service.addr.Port)) // add nat port mapping
	}
	go service.recvHandler()      // message receive handler
	go service.stallHandler()     // message response timeout handler
	go service.connectPeers()     // connect To network peers
	go service.addressHandler()   // request address From neighbor peers
	go service.heartBeatHandler() // start heartbeat handler

	service.isRunning = 1

	// debug p2p
	if service.config.DebugP2P {
		service.debugHandler = NewDebugHandler(service, service.center, service.config.DebugServer)
		service.debugHandler.Start()
	}
	return nil
}

// Stop stop p2p service
func (service *P2P) Stop() {
	// stop all peer.
	service.pendingPeers.Range(
		func(key, value interface{}) bool {
			peer := value.(*Peer)
			peer.Stop()
			return true
		},
	)
	service.outbountPeers.Range(
		func(key, value interface{}) bool {
			peer := value.(*Peer)
			peer.Stop()
			return true
		},
	)
	service.inboundPeers.Range(
		func(key, value interface{}) bool {
			peer := value.(*Peer)
			peer.Stop()
			return true
		},
	)

	service.lock.Lock()
	if service.isRunning != 1 {
		log.Error("p2p already stopped")
	}
	close(service.quitChan)
	service.addrManager.Stop()
	if service.listener != nil {
		service.listener.Close()
	}

	service.isRunning = 0

	service.lock.Unlock()

	// stop debug handler if exist
	if service.debugHandler != nil {
		service.debugHandler.Stop()
	}
}

// listen To accept connection From inbound peer.
func (service *P2P) startListen(listener net.Listener) {
	for {
		// listen To accept new connection
		conn, err := listener.Accept()
		if err != nil || conn == nil {
			log.Error("encounter error when accepting the new connection: %v", err)
			break
		}

		// parse inbound connection's address
		log.Debug("accept a new connection From %v", conn.RemoteAddr())
		addr, err := common.ParseNetAddress(conn.RemoteAddr().String())
		if err != nil {
			log.Error("unrecognized peer address: %v", err)
			conn.Close()
			continue
		}

		// check num of the inbound peer
		if service.GetInBountPeersCount() > service.config.MaxConnInBound {
			conn.Close()
			continue
		}

		// init an inbound peer
		peer := NewInboundPeer(&service.PeerCom, addr, service.internalChan, conn)
		//peer := NewInboundPeer(service.addrManager.OurAddresses(), addr, service.internalChan, conn)
		err = service.addPendingPeer(peer)
		if err != nil {
			conn.Close()
			log.Debug("failed To add peer %s To pending queue, as:%v", peer.GetAddr().ToString(), err)
			continue
		}
		go service.initInboundPeer(peer)
	}
}

const (
	mapDesc           = "justitia nat port mapping"
	mapProtocol       = "tcp"
	mapLifeTimeout    = 20 * time.Minute
	mapUpdateInterval = 15 * time.Minute
)

// add a port mapping and keeps it alive until c is closed.
func (service *P2P) addPortMapping(localPort int) {
	m := nat.DiscoverUPnPDevice()
	if m == nil {
		log.Error("Couldn't find Internet Gateway Device")
		return
	}
	refresh := time.NewTimer(mapUpdateInterval)
	defer func() {
		log.Debug("Deleting port mapping")
		refresh.Stop()
		m.DeletePortMapping(mapProtocol, localPort, localPort)
	}()
	if err := m.AddPortMapping(mapProtocol, localPort, localPort, mapDesc, mapLifeTimeout); err != nil {
		log.Warn("Couldn't add port mapping, as: %v", err)
	} else {
		log.Info("Mapped network port")
	}
	for {
		select {
		case <-service.quitChan:
			return
		case <-refresh.C:
			log.Debug("Refreshing port mapping")
			if err := m.AddPortMapping(mapProtocol, localPort, localPort, mapDesc, mapLifeTimeout); err != nil {
				log.Warn("Couldn't add port mapping, as: ", err)
			}
			refresh.Reset(mapUpdateInterval)
		}
	}
}

// init inbound peer
func (service *P2P) initInboundPeer(peer *Peer) {
	defer service.removePendingPeer(peer)
	err := peer.Start()
	if err != nil {
		log.Info("failed to start inbound peer as: %v", err)
		return
	}
	service.addrManager.AddAddress(peer.GetAddr())
	service.addrManager.ResetAddressAttemptInfo(peer.GetAddr()) //reset local attemptinfo that has successfully connected inbound peer.
	if service.config.SeedMode {
		// seed node close the connection after sending address message to new peer
		addrs := service.addrManager.GetAddresses()
		addrMsg := &message.Addr{
			NetAddresses: addrs,
		}
		if err := service.sendMsgSync(peer, addrMsg); err != nil {
			log.Error("failed to send address message to peer %s, as: %v", peer.GetAddr().ToString(), err)
		}
		peer.Stop()
	} else {
		service.addInBoundPeer(peer)
	}
}

// add pending peer
func (service *P2P) addPendingPeer(peer *Peer) error {
	log.Info("add peer %s To pending queue", peer.GetAddr().IP)
	if _, ok := service.pendingPeers.LoadOrStore(peer.GetAddr().IP, peer); ok {
		return fmt.Errorf("peer %s already in our pending peer list", peer.GetAddr().IP)
	}
	return nil
}

// remove pending peer
func (service *P2P) removePendingPeer(peer *Peer) {
	log.Info("remove peer %s From pending queue", peer.GetAddr().IP)
	service.pendingPeers.Delete(peer.GetAddr().IP)
}

// add inbound peer
func (service *P2P) addInBoundPeer(peer *Peer) error {
	log.Info("add a new inbound peer %s", peer.GetAddr().ToString())
	return service.addPeer(true, peer)
}

// add outbound peer
func (service *P2P) addOutBoundPeer(peer *Peer) error {
	log.Info("add a new outbound peer %s", peer.GetAddr().ToString())
	return service.addPeer(false, peer)
}

// add peer
func (service *P2P) addPeer(inbound bool, peer *Peer) error {
	if inbound {
		if _, ok := service.inboundPeers.LoadOrStore(peer.GetAddr().ToString(), peer); ok {
			return fmt.Errorf("peer %s already in our inbound peer list", peer.GetAddr().ToString())
		}
	} else {
		if _, ok := service.outbountPeers.LoadOrStore(peer.GetAddr().ToString(), peer); ok {
			return fmt.Errorf("peer %s already in our outbound peer list", peer.GetAddr().ToString())
		}
	}
	service.center.Notify(types.EventAddPeer, peer.GetAddr())
	return nil
}

// handle stall detection of the message response
func (service *P2P) stallHandler() {
	stallTimer := time.NewTimer(stallTickInterval)
	defer stallTimer.Stop()

	pendingResponses := make(map[*common.NetAddress]map[message.MessageType]time.Time)
	for {
		//register pending response
		select {
		case msg := <-service.stallChan:
			if msg == nil {
				continue
			}
			if service.isOutMsg(msg) {
				if msg.Payload != nil {
					log.Debug("stall handler register a %v type message To peer %s", msg.Payload.MsgType(), msg.To.ToString())
					addPendingRespMsg(pendingResponses, msg)
				}
			} else {
				if msg.Payload == nil {
					log.Debug("stall handler receive a clear %s's pending response message", msg.From.ToString())
				} else {
					log.Debug("stall handler receive a %v type message From %s", msg.Payload.MsgType(), msg.From.ToString())
				}
				removePendingRespMsg(pendingResponses, msg)
			}
		case <-service.quitChan:
			return
		}

		// check timeout response
		select {
		case <-stallTimer.C:
			now := time.Now()
			timeOutAddrs := make([]*common.NetAddress, 0)
			for addr, pendings := range pendingResponses {
				for msgType, deadline := range pendings {
					if now.Before(deadline) {
						continue
					}
					log.Error("receive %v type message's From Peer %s timeout", msgType, addr.ToString())
					timeOutAddrs = append(timeOutAddrs, addr)
					service.stopPeer(addr)
					break
				}
			}
			for _, timeOutAddr := range timeOutAddrs {
				delete(pendingResponses, timeOutAddr)
			}
			stallTimer.Reset(stallTickInterval)
		case <-service.quitChan:
			return
		default:
		}
	}
}

// check whether msg is out message.
func (service *P2P) isOutMsg(msg *InternalMsg) bool {
	if msg.From == nil {
		return false
	}
	return service.addrManager.IsOurAddress(msg.From)
}

// add a message To pending response queue
func addPendingRespMsg(pendingQueue map[*common.NetAddress]map[message.MessageType]time.Time, msg *InternalMsg) {
	deadline := time.Now().Add(stallResponseTimeout)
	if pendingQueue[msg.To] == nil {
		pendingQueue[msg.To] = make(map[message.MessageType]time.Time)
	}
	if _, ok := pendingQueue[msg.To][msg.Payload.ResponseMsgType()]; !ok {
		pendingQueue[msg.To][msg.Payload.ResponseMsgType()] = deadline
	}
}

// remove message when receiving corresponding response.
func removePendingRespMsg(pendingQueue map[*common.NetAddress]map[message.MessageType]time.Time, msg *InternalMsg) {
	if pendingQueue[msg.From] != nil {
		if msg.Payload != nil {
			delete(pendingQueue[msg.From], msg.Payload.MsgType())
		} else {
			delete(pendingQueue, msg.From)
		}
	}
}

// connectPeers connect To peers in p2p network
func (service *P2P) connectPeers() {
	service.connectPersistentPeers()
	if "" == service.config.PersistentPeers && !service.config.DisableDNSSeed {
		service.connectDnsSeeds()
	}
	service.connectNormalPeers()
}

// connect To persistent peers
func (service *P2P) connectPersistentPeers() {
	if service.config.PersistentPeers != "" {
		peerAddres := strings.Split(service.config.PersistentPeers, ",")
		for _, peerAddr := range peerAddres {
			netAddr, err := common.ParseNetAddress(peerAddr)
			if err != nil {
				log.Warn("invalid persistent peer address")
				continue
			}
			if service.addrManager.IsOurAddress(netAddr) {
				continue
			}

			service.addrManager.AddAddress(netAddr) //record address
			peer := NewOutboundPeer(&service.PeerCom, netAddr, true, service.internalChan)
			go service.connectPeer(peer)
		}
	}
}

// connect To dns seeds
func (service *P2P) connectDnsSeeds() {
	if "" != service.config.DNSSeeds {
		log.Info("connect to dns seeds")
		dnsSeeds := strings.Split(service.config.DNSSeeds, ",")
		for _, dnsSeed := range dnsSeeds {
			netAddr, err := common.ParseNetAddress(dnsSeed)
			if err != nil {
				log.Warn("invalid dns seed address")
				continue
			}
			if service.addrManager.IsOurAddress(netAddr) {
				continue
			}
			peer := NewOutboundPeer(&service.PeerCom, netAddr, false, service.internalChan)
			go service.connectPeer(peer)
		}
	}
}

// connect To dns seeds
func (service *P2P) connectNormalPeers() {
	log.Info("start connection To normal peers")
	reconnectInterval := 2 * time.Minute
	timer := time.NewTimer(0)
	defer timer.Stop()
	// random select peer To connect
	attemptTimes := 30 * (service.config.MaxConnOutBound - service.GetOutBountPeersCount())
	for {
		//wait for time out
		select {
		case <-timer.C:
		case <-service.quitChan:
			return
		}

		log.Info("start to connect to normal peers, current peer num: inbound-%d, outbound-%d.", service.GetInBountPeersCount(), service.GetOutBountPeersCount())
		if service.addrManager.GetAddressCount()-len(service.GetPeers()) < service.config.MaxConnOutBound {
			for _, addr := range service.addrManager.GetAllAddress() {
				if service.containsPeer(addr) {
					log.Debug("peer with addr %s already in our neighbor list", addr.ToString())
					continue
				}
				log.Info("start connecting To peer %s", addr.ToString())
				service.addrManager.UpdateAddressAttemptInfo(addr)
				peer := NewOutboundPeer(&service.PeerCom, addr, false, service.internalChan)
				go service.connectPeer(peer)
			}
		} else {
			// connect To peer
			for i := 0; i <= attemptTimes; i++ {
				if service.GetOutBountPeersCount() >= service.config.MaxConnOutBound || service.addrManager.GetAddressCount() <= service.GetOutBountPeersCount() {
					break
				}
				addr, err := service.addrManager.GetAddress()
				if err != nil {
					break
				}
				if service.containsPeer(addr) {
					continue
				}
				log.Info("start connecting To peer %s", addr.ToString())
				service.addrManager.UpdateAddressAttemptInfo(addr)
				peer := NewOutboundPeer(&service.PeerCom, addr, false, service.internalChan)
				go service.connectPeer(peer)
			}
		}

		//reset timer
		timer.Reset(reconnectInterval)
	}
}

// check whether peer with this address have existed in the neighbor list
func (service *P2P) containsPeer(addr *common.NetAddress) bool {
	if _, ok := service.pendingPeers.Load(addr.IP); ok {
		return true
	}
	if _, ok := service.outbountPeers.Load(addr.ToString()); ok {
		return true
	}
	if _, ok := service.inboundPeers.Load(addr.ToString()); ok {
		return true
	}
	return false
}

// connect To a peer
func (service *P2P) connectPeer(peer *Peer) {
RETRY:
	err := service.addPendingPeer(peer)
	if err != nil {
		log.Debug("failed To add peer %s To pending list, as: %v", peer.GetAddr().ToString(), err)
		return
	} else {
		err = peer.Start()
	}
	if err != nil {
		service.removePendingPeer(peer)
		log.Info("failed To connect To peer %s, as: %v", peer.GetAddr().ToString(), err)
		if peer.IsPersistent() {
			timer := time.NewTimer(persistentPeerRetryInterval)
			select {
			case <-timer.C:
				timer.Stop()
				goto RETRY
			case <-service.quitChan:
				timer.Stop()
				return
			}
		}
	} else {
		if service.config.SeedMode {
			addReq := &message.AddrReq{}
			service.sendMsgAsync(peer, addReq)
		}
		service.addOutBoundPeer(peer)
		service.removePendingPeer(peer)
	}
}

// stop the peer with specified address
func (service *P2P) stopPeer(addr *common.NetAddress) {
	if value, ok := service.pendingPeers.Load(addr.IP); ok {
		peer := value.(*Peer)
		peer.Stop()
		service.pendingPeers.Delete(addr.IP)
		service.center.Notify(types.EventRemovePeer, addr)
	}
	if value, ok := service.inboundPeers.Load(addr.ToString()); ok {
		peer := value.(*Peer)
		peer.Stop()
		service.inboundPeers.Delete(addr.ToString())
		service.center.Notify(types.EventRemovePeer, addr)
	}
	if value, ok := service.outbountPeers.Load(addr.ToString()); ok {
		peer := value.(*Peer)
		peer.Stop()
		service.outbountPeers.Delete(addr.ToString())
		service.center.Notify(types.EventRemovePeer, addr)
	}
}

// clear all pending response from this peer.
func (service *P2P) clearPendingResponse(peer *Peer) {
	cMsg := &InternalMsg{
		From:    peer.GetAddr(),
		Payload: nil,
	}
	service.stallChan <- cMsg
}

// addresses handler(request more addresses From neighbor peers)
func (service *P2P) addressHandler() {
	retryInterval := 30 * time.Second
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
		case <-service.quitChan:
			return
		}

		// get more address
		if service.addrManager.NeedMoreAddrs() {
			peers := service.GetPeers()
			if peers != nil && len(peers) > 0 {
				addReq := &message.AddrReq{}
				service.sendMsgAsync(peers[rand.Intn(len(peers))], addReq)
			}
		}
		timer.Reset(retryInterval)
	}
}

// receive handler (receive message From neighbor peers)
func (service *P2P) recvHandler() {
	for {
		select {
		case msg := <-service.internalChan:
			log.Debug("Server receive a message From %s", msg.From.ToString())
			service.stallChan <- msg
			switch msg.Payload.(type) {
			case *peerDisconnecMsg:
				service.stopPeer(msg.From)
			case *message.PingMsg:
				pingMsg := &message.PongMsg{
					State: LocalState(),
				}
				peer := service.GetPeerByAddress(msg.From)
				if peer != nil {
					service.sendMsgAsync(peer, pingMsg)
				}
			case *message.PongMsg:
				peer := service.GetPeerByAddress(msg.From)
				if peer != nil {
					peer.SetState(msg.Payload.(*message.PongMsg).State)
				}
			case *message.AddrReq:
				addrs := service.addrManager.GetAddresses()
				addrMsg := &message.Addr{
					NetAddresses: addrs,
				}
				peer := service.GetPeerByAddress(msg.From)
				if peer != nil {
					service.sendMsgAsync(peer, addrMsg)
				}
			case *message.Addr:
				addrMsg := msg.Payload.(*message.Addr)
				service.addrManager.AddAddresses(addrMsg.NetAddresses)
				if service.config.SeedMode {
					service.stopPeer(msg.From)
				}
			default:
				service.msgChan <- msg
				if service.config.DebugP2P {
					service.center.Notify(types.EventRecvNewMsg, msg)
				}
			}
		case <-service.quitChan:
			return
		}
	}
}

// send hear beat message periodically
func (service *P2P) heartBeatHandler() {
	timer := time.NewTimer(heartBeatInterval)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			pingMsg := &message.PingMsg{
				State: 1,
			}
			service.BroadCast(pingMsg)
		case <-service.quitChan:
			return
		}
		timer.Reset(heartBeatInterval)
	}
}

// BroadCast broad cast message To all neighbor peers
func (service *P2P) BroadCast(msg message.Message) {
	log.Debug("broadcas message (type: %v, id: %x) to neighbors", msg.MsgType(), msg.MsgId())
	service.outbountPeers.Range(
		func(key, value interface{}) bool {
			peer := value.(*Peer)
			if !peer.KnownMsg(msg) {
				go service.sendMsgAsync(peer, msg)
			}
			return true
		},
	)
	service.inboundPeers.Range(
		func(key, value interface{}) bool {
			peer := value.(*Peer)
			if !peer.KnownMsg(msg) {
				go service.sendMsgAsync(peer, msg)
			}
			return true
		},
	)
	if service.config.DebugP2P {
		imsg := &InternalMsg{
			From:    service.addrManager.OurAddresses()[0],
			Payload: msg,
		}
		service.center.Notify(types.EventBroadCastMsg, imsg)
	}
}

// SendMsg send message to a peer
func (service *P2P) SendMsg(peerAddr *common.NetAddress, msg message.Message) error {
	peer := service.GetPeerByAddress(peerAddr)
	if peer == nil {
		log.Error("no active peer with address %s", peerAddr.ToString())
		return fmt.Errorf("no active peer with address %s", peerAddr.ToString())
	}
	return service.sendMsgAsync(peer, msg)
}

// MessageChan get p2p's message channel, (Messages sent To the server will eventually be placed in the message channel)
func (service *P2P) MessageChan() <-chan *InternalMsg {
	log.Debug("get p2p's message chan")
	return service.msgChan
}

// Gather gather newest data From p2p network
func (service *P2P) Gather(peerFilter PeerFilter, reqMsg message.Message) error {
	if atomic.LoadInt32(&service.isRunning) != 1 {
		log.Error("P2P have not been started yet")
		return fmt.Errorf("P2P have not been started yet")
	}
	reqPeers := make([]*Peer, 0)
	peers := service.GetPeers()
	for _, peer := range peers {
		if peerFilter(peer.GetState()) {
			reqPeers = append(reqPeers, peer)
		}
	}

	if len(reqPeers) <= 0 {
		return errors.New("no suitable peer")
	}

	for _, peer := range peers {
		if peerFilter(peer.GetState()) {
			service.sendMsgAsync(peer, reqMsg)
		}
	}
	return nil
}

// sendMsgAsync send message asynchronously To a peer.
func (service *P2P) sendMsgAsync(peer *Peer, msg message.Message) error {
	return service.sendMsg(peer, msg, false)
}

// sendMsgSync send message synchronously To a peer.
func (service *P2P) sendMsgSync(peer *Peer, msg message.Message) error {
	return service.sendMsg(peer, msg, true)
}

// sendMsg send message To a peer.
func (service *P2P) sendMsg(peer *Peer, msg message.Message, sync bool) error {
	log.Debug("send message (type: %v, id: %x) to peer %s", msg.MsgType(), msg.MsgId(), peer.GetAddr().ToString())
	message := &InternalMsg{
		service.addrManager.OurAddresses()[0],
		peer.GetAddr(),
		msg,
		nil,
	}
	if sync {
		message.RespTo = make(chan interface{})
	}
	peer.SendMsg(message)
	service.registerPendingResp(message)

	if message.RespTo != nil {
		timer := time.NewTimer(30 * time.Second)
		defer timer.Stop()
		select {
		case resp := <-message.RespTo:
			if _, ok := resp.(error); ok {
				return resp.(error)
			}
		case <-timer.C:
			return fmt.Errorf("receive message(%v) response from %s timeout", msg.MsgType(), peer.GetAddr().ToString())
		}
	}
	return nil
}

// register need response message To pending response queue
func (service *P2P) registerPendingResp(msg *InternalMsg) {
	//check whether message need response
	if msg.Payload.ResponseMsgType() != message.NIL {
		service.stallChan <- msg
	}
}

// GetOutBountPeersCount get out bount peer count
func (service *P2P) GetOutBountPeersCount() int {
	count := 0
	service.outbountPeers.Range(
		func(key, value interface{}) bool {
			count++
			return true
		},
	)
	return count
}

// GetOutBountPeersCount get out bount peer count
func (service *P2P) GetInBountPeersCount() int {
	count := 0
	service.inboundPeers.Range(
		func(key, value interface{}) bool {
			count++
			return true
		},
	)
	return count
}

// GetPeers get service's inbound peers and outbound peers
func (service *P2P) GetPeers() []*Peer {
	peers := make([]*Peer, 0)
	service.outbountPeers.Range(
		func(key, value interface{}) bool {
			peer := value.(*Peer)
			peers = append(peers, peer)
			return true
		},
	)
	service.inboundPeers.Range(
		func(key, value interface{}) bool {
			peer := value.(*Peer)
			peers = append(peers, peer)
			return true
		},
	)
	return peers
}

// GetPeerByAddress get a peer by net address
func (service *P2P) GetPeerByAddress(addr *common.NetAddress) *Peer {
	if value, ok := service.inboundPeers.Load(addr.ToString()); ok {
		return value.(*Peer)
	}
	if value, ok := service.outbountPeers.Load(addr.ToString()); ok {
		return value.(*Peer)
	}
	return nil
}

//	used to verify peer compatibility
func (service *P2P) onVersion(versionMsg *message.Version) error {
	if !version.Accept(versionMsg.Version) {
		return errors.New("Version not compatible with the server ")
	}
	if versionMsg.Service != service.service {
		return errors.New("Service type not compatible with the server ")
	}
	return nil
}

// get local state
func (service *P2P) getLocalState() uint64 {
	//TODO get local state
	return 1
}
