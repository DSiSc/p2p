package p2p

import (
	"errors"
	"fmt"
	"github.com/DSiSc/craft/log"
	"github.com/DSiSc/p2p/common"
	"github.com/DSiSc/p2p/config"
	"github.com/DSiSc/p2p/message"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const (
	MAX_BUF_LEN    = 1024 * 256 //the maximum buffer To receive message
	WRITE_DEADLINE = 60         //deadline of conn write

)

// PeerCom provides the basic information of a peer
type PeerCom struct {
	version    string             // version info
	addr       *common.NetAddress // peer address
	state      uint64             //current state of this peer
	outBound   atomic.Value       // whether peer is out bound peer
	persistent bool               // whether peer is persistent peer
	service    config.ServiceFlag // service peer supported
}

// Peer represent the peer
type Peer struct {
	PeerCom
	serverInfo   *PeerCom
	conn         *PeerConn //connection To this peer
	internalChan chan message.Message
	sendChan     chan *InternalMsg
	recvChan     chan<- *InternalMsg
	quitChan     chan interface{}
	lock         sync.RWMutex
	isRunning    int32
	knownMsgs    *common.RingBuffer
}

// NewInboundPeer new inbound peer instance
func NewInboundPeer(serverInfo *PeerCom, addr *common.NetAddress, msgChan chan<- *InternalMsg, conn net.Conn) *Peer {
	return newPeer(serverInfo, addr, false, false, msgChan, conn)
}

// NewInboundPeer new outbound peer instance
func NewOutboundPeer(serverInfo *PeerCom, addr *common.NetAddress, persistent bool, msgChan chan<- *InternalMsg) *Peer {
	return newPeer(serverInfo, addr, true, persistent, msgChan, nil)
}

// create a peer instance.
func newPeer(serverInfo *PeerCom, addr *common.NetAddress, outBound, persistent bool, msgChan chan<- *InternalMsg, conn net.Conn) *Peer {
	peer := &Peer{
		PeerCom: PeerCom{
			addr:       addr,
			persistent: persistent,
		},
		serverInfo:   serverInfo,
		internalChan: make(chan message.Message),
		sendChan:     make(chan *InternalMsg),
		recvChan:     msgChan,
		quitChan:     make(chan interface{}),
		knownMsgs:    common.NewRingBuffer(1024),
		isRunning:    0,
	}
	peer.outBound.Store(outBound)
	if !outBound && conn != nil {
		peer.conn = NewPeerConn(conn, peer.internalChan)
	}
	return peer
}

// Start connect To peer and send message To each other
func (peer *Peer) Start() error {
	peer.lock.Lock()
	defer peer.lock.Unlock()
	if peer.isRunning != 0 {
		log.Error("peer %s has been started", peer.addr.ToString())
		return fmt.Errorf("peer %s has been started", peer.addr.ToString())
	}

	if peer.outBound.Load().(bool) {
		log.Info("Start outbound peer %s", peer.addr.ToString())
		err := peer.initConn()
		if err != nil {
			return err
		}
		peer.conn.Start()
		err = peer.handShakeWithOutBoundPeer()
		if err != nil {
			log.Info("failed to hand shake with outbound peer %s, as: %v", peer.addr.ToString(), err)
			peer.conn.Stop()
			return err
		}
	} else {
		log.Info("Start inbound peer %s", peer.addr.ToString())
		if peer.conn == nil {
			return errors.New("have no established connection")
		}
		peer.conn.Start()
		err := peer.handShakeWithInBoundPeer()
		if err != nil {
			log.Info("failed to hand shake with inbound peer %s, as: %v", peer.addr.ToString(), err)
			peer.conn.Stop()
			return err
		}
	}

	go peer.recvHandler()
	go peer.sendHandler()
	peer.isRunning = 1
	return nil
}

// start handshake with outbound peer.
func (peer *Peer) handShakeWithOutBoundPeer() error {
	//send version message
	err := peer.sendVersionMessage()
	if err != nil {
		return err
	}

	// read version message
	err = peer.readVersionMessage()
	if err != nil {
		return err
	}

	// send version ack message
	err = peer.sendVersionAckMessage()
	if err != nil {
		return err
	}

	// read version ack message
	return peer.readVersionAckMessage()
}

// start handshake with inbound peer.
func (peer *Peer) handShakeWithInBoundPeer() error {
	// read version message
	err := peer.readVersionMessage()
	if err != nil {
		return err
	}

	//send version message
	err = peer.sendVersionMessage()
	if err != nil {
		return err
	}

	// read version ack message
	err = peer.readVersionAckMessage()
	if err != nil {
		return err
	}

	// send version ack message
	return peer.sendVersionAckMessage()
}

// send version message To this peer.
func (peer *Peer) sendVersionMessage() error {
	vmsg := &message.Version{
		Version: peer.serverInfo.version,
		PortMe:  peer.serverInfo.addr.Port,
		Service: peer.serverInfo.service,
	}
	return peer.conn.SendMessage(vmsg)
}

// send version ack message To this peer.
func (peer *Peer) sendVersionAckMessage() error {
	vackmsg := &message.VersionAck{}
	return peer.conn.SendMessage(vackmsg)
}

// read version message
func (peer *Peer) readVersionMessage() error {
	msg, err := peer.readMessageWithType(message.VERSION_TYPE)
	if err != nil {
		return err
	}
	vmsg := msg.(*message.Version)
	if vmsg.Service != peer.serverInfo.service {
		return errors.New("Incompatible service ")
	}
	if !peer.outBound.Load().(bool) {
		peer.addr.Port = vmsg.PortMe
	}
	return nil
}

// read version ack message
func (peer *Peer) readVersionAckMessage() error {
	_, err := peer.readMessageWithType(message.VERACK_TYPE)
	if err != nil {
		return err
	}
	return nil
}

// read specified type message From peer.
func (peer *Peer) readMessageWithType(msgType message.MessageType) (message.Message, error) {
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case msg := <-peer.internalChan:
		if msg.MsgType() == msgType {
			return msg, nil
		} else {
			log.Warn("error type message received From peer %s, expected: %v, actual: %v", peer.addr.ToString(), msgType, msg.MsgType())
			return nil, fmt.Errorf("error type message received From peer %s, expected: %v, actual: %v", peer.addr.ToString(), msgType, msg.MsgType())
		}
	case <-timer.C:
		log.Warn("read %v type message From peer %s time out", msgType, peer.addr.ToString())
		return nil, fmt.Errorf("read %v type message From peer %s time out", msgType, peer.addr.ToString())
	}
}

// Stop stop peer.
func (peer *Peer) Stop() {
	log.Info("Stop peer %s", peer.GetAddr().ToString())

	peer.lock.Lock()
	defer peer.lock.Unlock()
	if peer.isRunning == 0 {
		return
	}
	if peer.conn != nil {
		peer.conn.Stop()
	}
	close(peer.quitChan)
	peer.isRunning = 0
}

// initConnection init the connection To peer.
func (peer *Peer) initConn() error {
	log.Debug("start init the connection To peer %s", peer.addr.ToString())
	dialAddr := peer.addr.IP + ":" + strconv.Itoa(int(peer.addr.Port))
	conn, err := net.Dial("tcp", dialAddr)
	if err != nil {
		log.Info("failed To dial To peer %s, as : %v", peer.addr.ToString(), err)
		return fmt.Errorf("failed To dial To peer %s, as : %v", peer.addr.ToString(), err)
	}
	peer.conn = NewPeerConn(conn, peer.internalChan)
	return nil
}

// message receive handler
func (peer *Peer) recvHandler() {
	for {
		var msg message.Message
		select {
		case msg = <-peer.internalChan:
			log.Debug("receive %v type message From peer %s", msg.MsgType(), peer.GetAddr().ToString())
			if msg.MsgId() != message.EmptyHash {
				peer.knownMsgs.AddElement(msg.MsgId(), struct{}{})
			}
		case <-peer.quitChan:
			return
		}

		switch msg.(type) {
		case *message.Version:
			reject := &message.RejectMsg{
				Reason: "invalid message, as version messages can only be sent once ",
			}
			peer.conn.SendMessage(reject)
			peer.disconnectNotify(errors.New("receive an invalid message From remote"))
			return
		case *message.VersionAck:
			reject := &message.RejectMsg{
				Reason: "invalid message, as version ack messages can only be sent once ",
			}
			peer.conn.SendMessage(reject)
			peer.disconnectNotify(errors.New("receive an invalid message From remote"))
			return
		case *message.RejectMsg:
			rejectMsg := msg.(*message.RejectMsg)
			log.Error("receive a reject message From remote, reject reason: %s", rejectMsg.Reason)
			peer.disconnectNotify(errors.New(rejectMsg.Reason))
			return
		default:
			imsg := &InternalMsg{
				From:    peer.addr,
				To:      peer.serverInfo.addr,
				Payload: msg,
			}
			peer.receivedMsg(imsg)
			log.Debug("peer %s send %v type message To message channel", peer.GetAddr().ToString(), msg.MsgType())
		}
	}
}

// message send handler
func (peer *Peer) sendHandler() {
	for {
		select {
		case msg := <-peer.sendChan:
			if msg.Payload.MsgId() != message.EmptyHash {
				peer.knownMsgs.AddElement(msg.Payload.MsgId(), struct{}{})
			}
			err := peer.conn.SendMessage(msg.Payload)
			if msg.RespTo != nil {
				if err != nil {
					msg.RespTo <- err
				} else {
					msg.RespTo <- nilError
				}
			}
		case <-peer.quitChan:
			return
		}
	}
}

// IsPersistent return true if this peer is a persistent peer
func (peer *Peer) IsPersistent() bool {
	peer.lock.RLock()
	defer peer.lock.RUnlock()
	return peer.persistent
}

// GetAddr get peer's address
func (peer *Peer) GetAddr() *common.NetAddress {
	peer.lock.RLock()
	defer peer.lock.RUnlock()
	return peer.addr
}

// CurrentState get current state of this peer.
func (peer *Peer) CurrentState() uint64 {
	peer.lock.RLock()
	defer peer.lock.RUnlock()
	return peer.state
}

// Channel get peer's send channel
func (peer *Peer) SendMsg(msg *InternalMsg) error {
	select {
	case peer.sendChan <- msg:
		return nil
	case <-peer.quitChan:
		return fmt.Errorf("peer %s have stopped", peer.GetAddr().ToString())
	}
}

// SetState update peer's state
func (peer *Peer) SetState(state uint64) {
	peer.lock.Lock()
	defer peer.lock.Unlock()
	peer.state = state
}

// SetState update peer's state
func (peer *Peer) GetState() uint64 {
	peer.lock.RLock()
	defer peer.lock.RUnlock()
	return peer.state
}

// KnownMsg check whether the peer already known this message
func (peer *Peer) KnownMsg(msg message.Message) bool {
	return peer.knownMsgs.Exist(msg.MsgId())
}

// IsOutBound check whether the peer is outbound peer.
func (peer *Peer) IsOutBound() bool {
	return peer.outBound.Load().(bool)
}

//disconnectNotify push disconnect msg To channel
func (peer *Peer) disconnectNotify(err error) {
	log.Debug("[p2p]call disconnectNotify for %s, as: %v", peer.GetAddr().ToString(), err)
	disconnectMsg := &peerDisconnecMsg{
		err,
	}
	msg := &InternalMsg{
		From:    peer.addr,
		To:      peer.serverInfo.addr,
		Payload: disconnectMsg,
	}
	peer.receivedMsg(msg)
}

// received a message from remote
func (peer *Peer) receivedMsg(msg *InternalMsg) {
	select {
	case peer.recvChan <- msg:
	case <-peer.quitChan:
		log.Warn("Peer have been closed")
	}
}
