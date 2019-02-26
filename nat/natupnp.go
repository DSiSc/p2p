package nat

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/huin/goupnp"
	"github.com/huin/goupnp/dcps/internetgateway1"
	"github.com/huin/goupnp/dcps/internetgateway2"
)

const soapRequestTimeout = 3 * time.Second

// upnpClient is the generic abstraction of the upnp InternetGatewayDevice client.
type upnpClient interface {
	GetExternalIPAddress() (string, error)
	AddPortMapping(string, uint16, string, uint16, string, bool, string, uint32) error
	DeletePortMapping(string, uint16, string) error
	GetNATRSIPStatus() (sip bool, nat bool, err error)
}

// UpnpNat used to add nat port mapping through via upnp protocol
type UpnpNat struct {
	device  *goupnp.RootDevice
	service string
	client  upnpClient
}

// GetExternalIPAddress get the external address.
func (n *UpnpNat) GetExternalIPAddress() (addr net.IP, err error) {
	ipString, err := n.client.GetExternalIPAddress()
	if err != nil {
		return nil, err
	}
	ip := net.ParseIP(ipString)
	if ip == nil {
		return nil, errors.New("bad IP in response")
	}
	return ip, nil
}

// AddPortMapping add port mapping
func (n *UpnpNat) AddPortMapping(protocol string, extport, intport int, desc string, lifetime time.Duration) error {
	ip, err := n.getInternalAddress()
	if err != nil {
		return nil
	}
	protocol = strings.ToUpper(protocol)
	lifetimeS := uint32(lifetime / time.Second)
	n.DeletePortMapping(protocol, extport, intport)
	return n.client.AddPortMapping("", uint16(extport), protocol, uint16(intport), ip.String(), true, desc, lifetimeS)
}

// DeletePortMapping delete port mapping
func (n *UpnpNat) DeletePortMapping(protocol string, extport, intport int) error {
	return n.client.DeletePortMapping("", uint16(extport), strings.ToUpper(protocol))
}

// get internal address
func (n *UpnpNat) getInternalAddress() (net.IP, error) {
	devaddr, err := net.ResolveUDPAddr("udp4", n.device.URLBase.Host)
	if err != nil {
		return nil, err
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, err
		}
		for _, addr := range addrs {
			if x, ok := addr.(*net.IPNet); ok && x.Contains(devaddr.IP) {
				return x.IP, nil
			}
		}
	}
	return nil, fmt.Errorf("could not find local address in same net as %v", devaddr)
}

// DiscoverUPnPDevice searches for Internet Gateway Devices
// and returns the first one it can find on the local network.
func DiscoverUPnPDevice() *UpnpNat {
	found := make(chan *UpnpNat, 2)
	// InternetGatewayDevice-v1
	go discover(found, internetgateway1.URN_WANConnectionDevice_1, func(dev *goupnp.RootDevice, sc goupnp.ServiceClient) *UpnpNat {
		switch sc.Service.ServiceType {
		case internetgateway1.URN_WANIPConnection_1:
			return &UpnpNat{dev, "IGDv1-IP1", &internetgateway1.WANIPConnection1{ServiceClient: sc}}
		case internetgateway1.URN_WANPPPConnection_1:
			return &UpnpNat{dev, "IGDv1-PPP1", &internetgateway1.WANPPPConnection1{ServiceClient: sc}}
		}
		return nil
	})
	// InternetGatewayDevice-v2
	go discover(found, internetgateway2.URN_WANConnectionDevice_2, func(dev *goupnp.RootDevice, sc goupnp.ServiceClient) *UpnpNat {
		switch sc.Service.ServiceType {
		case internetgateway2.URN_WANIPConnection_1:
			return &UpnpNat{dev, "IGDv2-IP1", &internetgateway2.WANIPConnection1{ServiceClient: sc}}
		case internetgateway2.URN_WANIPConnection_2:
			return &UpnpNat{dev, "IGDv2-IP2", &internetgateway2.WANIPConnection2{ServiceClient: sc}}
		case internetgateway2.URN_WANPPPConnection_1:
			return &UpnpNat{dev, "IGDv2-PPP1", &internetgateway2.WANPPPConnection1{ServiceClient: sc}}
		}
		return nil
	})
	for i := 0; i < cap(found); i++ {
		if c := <-found; c != nil {
			return c
		}
	}
	return nil
}

// finds devices matching the given target and calls matcher for all
// advertised services of each device. The first non-nil service found
// is sent into out. If no service matched, nil is sent.
func discover(out chan<- *UpnpNat, target string, matcher func(*goupnp.RootDevice, goupnp.ServiceClient) *UpnpNat) {
	devs, err := goupnp.DiscoverDevices(target)
	if err != nil {
		out <- nil
		return
	}
	found := false
	for i := 0; i < len(devs) && !found; i++ {
		if devs[i].Root == nil {
			continue
		}
		devs[i].Root.Device.VisitServices(func(service *goupnp.Service) {
			if found {
				return
			}
			// check for a matching InternetGatewayDevice service
			sc := goupnp.ServiceClient{
				SOAPClient: service.NewSOAPClient(),
				RootDevice: devs[i].Root,
				Location:   devs[i].Location,
				Service:    service,
			}
			sc.SOAPClient.HTTPClient.Timeout = soapRequestTimeout
			upnp := matcher(devs[i].Root, sc)
			if upnp == nil {
				return
			}
			// check whether port mapping is enabled
			if _, nat, err := upnp.client.GetNATRSIPStatus(); err != nil || !nat {
				return
			}
			out <- upnp
			found = true
		})
	}
	if !found {
		out <- nil
	}
}
