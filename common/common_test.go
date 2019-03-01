package common

import (
	"github.com/DSiSc/craft/types"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

var MockHash = types.Hash{
	0x1d, 0xcf, 0x7, 0xba, 0xfc, 0x42, 0xb0, 0x8d, 0xfd, 0x23, 0x9c, 0x45, 0xa4, 0xb9, 0x38, 0xd,
	0x8d, 0xfe, 0x5d, 0x6f, 0xa7, 0xdb, 0xd5, 0x50, 0xc9, 0x25, 0xb1, 0xb3, 0x4, 0xdc, 0xc5, 0x1c,
}

func MockBlock() *types.Block {
	return &types.Block{
		Header: &types.Header{
			ChainID:       1,
			PrevBlockHash: MockHash,
			StateRoot:     MockHash,
			TxRoot:        MockHash,
			ReceiptsRoot:  MockHash,
			Height:        1,
			Timestamp:     uint64(time.Date(2018, time.August, 28, 0, 0, 0, 0, time.UTC).Unix()),
		},
		Transactions: make([]*types.Transaction, 0),
	}
}

var MockBlockHash = types.Hash{
	0x9, 0x99, 0xfd, 0xff, 0x97, 0x34, 0xff, 0xa9, 0xda, 0x64, 0x69, 0xcb, 0x62, 0x6d, 0x7a, 0xec, 0x1c, 0xa1, 0xb2, 0xbf, 0x50, 0x5b, 0x71, 0x6, 0x3e, 0x20, 0x5b, 0x66, 0xb2, 0xd4, 0xbf, 0xb1}

func TestHeaderHash(t *testing.T) {
	block := MockBlock()
	hash := HeaderHash(block)
	assert.Equal(t, MockBlockHash, hash)
}

func TestHexToAddress(t *testing.T) {
	addHex := "333c3310824b7c685133f2bedb2ca4b8b4df633d"
	address := HexToAddress(addHex)
	b := types.Address{
		0x33, 0x3c, 0x33, 0x10, 0x82, 0x4b, 0x7c, 0x68, 0x51, 0x33,
		0xf2, 0xbe, 0xdb, 0x2c, 0xa4, 0xb8, 0xb4, 0xdf, 0x63, 0x3d,
	}
	assert.Equal(t, b, address)
}

func TestHex2Bytes(t *testing.T) {
	addHex := "333c3310824b7c685133f2bedb2ca4b8b4df633d"
	address := Hex2Bytes(addHex)
	b := []byte{
		0x33, 0x3c, 0x33, 0x10, 0x82, 0x4b, 0x7c, 0x68, 0x51, 0x33,
		0xf2, 0xbe, 0xdb, 0x2c, 0xa4, 0xb8, 0xb4, 0xdf, 0x63, 0x3d,
	}
	assert.Equal(t, b, address)
}

func TestFromHex(t *testing.T) {
	addHex := "333c3310824b7c685133f2bedb2ca4b8b4df633d"
	address := FromHex(addHex)
	b := []byte{
		0x33, 0x3c, 0x33, 0x10, 0x82, 0x4b, 0x7c, 0x68, 0x51, 0x33,
		0xf2, 0xbe, 0xdb, 0x2c, 0xa4, 0xb8, 0xb4, 0xdf, 0x63, 0x3d,
	}
	assert.Equal(t, b, address)
}
