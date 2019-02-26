package nat

import (
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	dev := NewIGDDev()
	if err := dev.Listen(); err != nil {
		os.Exit(1)
	}
	dev.Serve()
	defer dev.Close()
	m.Run()
}

// test discover device
func TestDiscoverUPnPDevice(t *testing.T) {
	assert := assert.New(t)
	dev := DiscoverUPnPDevice()
	assert.NotNil(dev)
}

// test add port mapping
func TestUpnpNat_AddPortMapping(t *testing.T) {
	assert := assert.New(t)
	dev := DiscoverUPnPDevice()
	assert.NotNil(dev)
	err := dev.AddPortMapping("tcp", 8080, 8080, "justitia p2p", time.Minute)
	assert.Nil(err)
}

// test get external ip.
func TestUpnpNat_GetExternalIPAddress(t *testing.T) {
	assert := assert.New(t)
	dev := DiscoverUPnPDevice()
	assert.NotNil(dev)
	err := dev.AddPortMapping("tcp", 8080, 8080, "justitia p2p", time.Minute)
	assert.Nil(err)
	extIp, err := dev.GetExternalIPAddress()
	assert.Nil(err)
	assert.Equal("192.168.1.1", extIp.String())
}

// test delete port mapping
func TestUpnpNat_DeletePortMapping(t *testing.T) {
	assert := assert.New(t)
	dev := DiscoverUPnPDevice()
	assert.NotNil(dev)
	err := dev.AddPortMapping("tcp", 8080, 8080, "justitia p2p", time.Minute)
	assert.Nil(err)
	err = dev.DeletePortMapping("tcp", 8080, 8080)
	assert.Nil(err)
}
