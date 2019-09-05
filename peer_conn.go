package p2p

import (
	"bufio"
	"github.com/DSiSc/craft/log"
	"github.com/DSiSc/p2p/message"
	"net"
	"sync"
	"time"
)

// PeerConn is the abstract of the net.Conn To this peer.
type PeerConn struct {
	conn      net.Conn //connection To this peer
	recvChan  chan message.Message
	quitChan  chan interface{}
	lock      sync.RWMutex
	isRunning int32
}

// NewPeerConn create a PeerConn instance
func NewPeerConn(conn net.Conn, recvChan chan message.Message) *PeerConn {
	return &PeerConn{
		conn:      conn,
		recvChan:  recvChan,
		quitChan:  make(chan interface{}),
		isRunning: 0,
	}
}

// Start start PeerConn
// will start receive and send handler To handle the message From/To net.Conn
func (peerConn *PeerConn) Start() {
	peerConn.lock.Lock()
	defer peerConn.lock.Unlock()
	go peerConn.recvHandler()
	peerConn.isRunning = 1
}

// Stop stop PeerConn
func (peerConn *PeerConn) Stop() {
	peerConn.lock.Lock()
	defer peerConn.lock.Unlock()
	if peerConn.isRunning == 0 {
		return
	}
	close(peerConn.quitChan)
	peerConn.conn.Close()
}

// message receive handler
func (peerConn *PeerConn) recvHandler() {
	reader := bufio.NewReaderSize(peerConn.conn, MAX_BUF_LEN)
	for {
		// read new message From connection
		msg, err := message.ReadMessage(reader)
		if err != nil {
			log.Error("failed To read message From remote %s, as: %v", peerConn.conn.RemoteAddr().String(), err)
			peerConn.disconnectNotify(err)
			return
		}
		peerConn.receivedMsg(msg)
	}
}

// SendMessage message To this PeerConn.
func (peerConn *PeerConn) SendMessage(msg message.Message) error {
	log.Debug("send message (type:%d, id: %x) To remote %s", msg.MsgType(), msg.MsgId(), peerConn.conn.RemoteAddr().String())
	buf, err := message.EncodeMessage(msg)
	if err != nil {
		log.Error("failed To encode message %v, as %v", msg, err)
		return err
	}

	peerConn.conn.SetWriteDeadline(time.Now().Add(time.Duration(WRITE_DEADLINE) * time.Second))
	_, err = peerConn.conn.Write(buf)
	if err != nil {
		log.Error("failed To send raw message To remote %s, as: %v", peerConn.conn.RemoteAddr().String(), err)
		return err
	}
	return nil
}

//disconnectNotify push disconnect msg To channel
func (peerConn *PeerConn) disconnectNotify(err error) {
	log.Debug("call disconnectNotify for %s, as: %v", peerConn.conn.RemoteAddr().String(), err)
	disconnectMsg := &peerDisconnecMsg{
		err,
	}
	peerConn.receivedMsg(disconnectMsg)
}

// received a message from remote
func (peerConn *PeerConn) receivedMsg(msg message.Message) {
	log.Debug("received message (type: %v, id: %x) from remote %s", msg.MsgType(), msg.MsgId(), peerConn.conn.RemoteAddr().String())
	select {
	case peerConn.recvChan <- msg:
	case <-peerConn.quitChan:
		log.Warn("Peer Connection have been closed")
	}
}
