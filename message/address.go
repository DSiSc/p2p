package message

import (
	"github.com/DSiSc/p2p/common"
)

type AddrReq struct{}

func (this *AddrReq) MsgType() MessageType {
	return GETADDR_TYPE
}

func (this *AddrReq) ResponseMsgType() MessageType {
	return ADDR_TYPE
}

type Addr struct {
	NetAddresses []*common.NetAddress `json:"net_addresses"`
}

func (this *Addr) MsgType() MessageType {
	return ADDR_TYPE
}

func (this *Addr) ResponseMsgType() MessageType {
	return NIL
}
