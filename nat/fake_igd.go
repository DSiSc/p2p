package nat

import (
	"encoding/xml"
	"fmt"
	"github.com/huin/goupnp/httpu"
	"io"
	"net"
	"net/http"
	"strings"
)

// FakeIGD presents itself as a discoverable UPnP device which sends
// canned responses to HTTPU and HTTP requests.
type FakeIGD struct {
	listener      net.Listener
	mcastListener *net.UDPConn

	// This should be a complete HTTP response (including headers).
	// It is sent as the response to any sspd packet. Any occurrence
	// of "{{listenAddr}}" is replaced with the actual TCP Listen
	// address of the HTTP server.
	ssdpResp string
	// This one should contain XML payloads for all requests
	// performed. The keys contain method and path, e.g. "GET /foo/bar".
	// As with ssdpResp, "{{listenAddr}}" is replaced with the TCP
	// Listen address.
	httpResps map[string]string
}

// create a FakeIGD instance
func NewIGDDev() *FakeIGD {
	dev := &FakeIGD{
		ssdpResp: "HTTP/1.1 200 OK\r\n" +
			"Cache-Control: max-age=300\r\n" +
			"Date: Sun, 10 May 2015 10:05:33 GMT\r\n" +
			"Ext: \r\n" +
			"Location: http://{{listenAddr}}/InternetGatewayDevice.xml\r\n" +
			"Server: POSIX UPnP/1.0 DD-WRT Linux/V24\r\n" +
			"ST: urn:schemas-upnp-org:device:WANConnectionDevice:1\r\n" +
			"USN: uuid:CB2471CC-CF2E-9795-8D9C-E87B34C16800::urn:schemas-upnp-org:device:WANConnectionDevice:1\r\n" +
			"\r\n",
		httpResps: map[string]string{
			"GET /InternetGatewayDevice.xml": `
				 <?xml version="1.0"?>
				 <root xmlns="urn:schemas-upnp-org:device-1-0">
					 <specVersion>
						 <major>1</major>
						 <minor>0</minor>
					 </specVersion>
					 <device>
						 <deviceType>urn:schemas-upnp-org:device:InternetGatewayDevice:1</deviceType>
						 <manufacturer>DD-WRT</manufacturer>
						 <manufacturerURL>http://www.dd-wrt.com</manufacturerURL>
						 <modelDescription>Gateway</modelDescription>
						 <friendlyName>Asus RT-N16:DD-WRT</friendlyName>
						 <modelName>Asus RT-N16</modelName>
						 <modelNumber>V24</modelNumber>
						 <serialNumber>0000001</serialNumber>
						 <modelURL>http://www.dd-wrt.com</modelURL>
						 <UDN>uuid:A13AB4C3-3A14-E386-DE6A-EFEA923A06FE</UDN>
						 <serviceList>
							 <service>
								 <serviceType>urn:schemas-upnp-org:service:Layer3Forwarding:1</serviceType>
								 <serviceId>urn:upnp-org:serviceId:L3Forwarding1</serviceId>
								 <SCPDURL>/x_layer3forwarding.xml</SCPDURL>
								 <controlURL>/control?Layer3Forwarding</controlURL>
								 <eventSubURL>/event?Layer3Forwarding</eventSubURL>
							 </service>
						 </serviceList>
						 <deviceList>
							 <device>
								 <deviceType>urn:schemas-upnp-org:device:WANDevice:1</deviceType>
								 <friendlyName>WANDevice</friendlyName>
								 <manufacturer>DD-WRT</manufacturer>
								 <manufacturerURL>http://www.dd-wrt.com</manufacturerURL>
								 <modelDescription>Gateway</modelDescription>
								 <modelName>router</modelName>
								 <modelURL>http://www.dd-wrt.com</modelURL>
								 <UDN>uuid:48FD569B-F9A9-96AE-4EE6-EB403D3DB91A</UDN>
								 <serviceList>
									 <service>
										 <serviceType>urn:schemas-upnp-org:service:WANCommonInterfaceConfig:1</serviceType>
										 <serviceId>urn:upnp-org:serviceId:WANCommonIFC1</serviceId>
										 <SCPDURL>/x_wancommoninterfaceconfig.xml</SCPDURL>
										 <controlURL>/control?WANCommonInterfaceConfig</controlURL>
										 <eventSubURL>/event?WANCommonInterfaceConfig</eventSubURL>
									 </service>
								 </serviceList>
								 <deviceList>
									 <device>
										 <deviceType>urn:schemas-upnp-org:device:WANConnectionDevice:1</deviceType>
										 <friendlyName>WAN Connection Device</friendlyName>
										 <manufacturer>DD-WRT</manufacturer>
										 <manufacturerURL>http://www.dd-wrt.com</manufacturerURL>
										 <modelDescription>Gateway</modelDescription>
										 <modelName>router</modelName>
										 <modelURL>http://www.dd-wrt.com</modelURL>
										 <UDN>uuid:CB2471CC-CF2E-9795-8D9C-E87B34C16800</UDN>
										 <serviceList>
											 <service>
												 <serviceType>urn:schemas-upnp-org:service:WANIPConnection:1</serviceType>
												 <serviceId>urn:upnp-org:serviceId:WANIPConn1</serviceId>
												 <SCPDURL>/x_wanipconnection.xml</SCPDURL>
												 <controlURL>/control?WANIPConnection</controlURL>
												 <eventSubURL>/event?WANIPConnection</eventSubURL>
											 </service>
										 </serviceList>
									 </device>
								 </deviceList>
							 </device>
							 <device>
								 <deviceType>urn:schemas-upnp-org:device:LANDevice:1</deviceType>
								 <friendlyName>LANDevice</friendlyName>
								 <manufacturer>DD-WRT</manufacturer>
								 <manufacturerURL>http://www.dd-wrt.com</manufacturerURL>
								 <modelDescription>Gateway</modelDescription>
								 <modelName>router</modelName>
								 <modelURL>http://www.dd-wrt.com</modelURL>
								 <UDN>uuid:04021998-3B35-2BDB-7B3C-99DA4435DA09</UDN>
								 <serviceList>
									 <service>
										 <serviceType>urn:schemas-upnp-org:service:LANHostConfigManagement:1</serviceType>
										 <serviceId>urn:upnp-org:serviceId:LANHostCfg1</serviceId>
										 <SCPDURL>/x_lanhostconfigmanagement.xml</SCPDURL>
										 <controlURL>/control?LANHostConfigManagement</controlURL>
										 <eventSubURL>/event?LANHostConfigManagement</eventSubURL>
									 </service>
								 </serviceList>
							 </device>
						 </deviceList>
						 <presentationURL>http://{{listenAddr}}</presentationURL>
					 </device>
				 </root>
			`,
			// The response to our GetNATRSIPStatus call. This
			// particular implementation has a bug where the elements
			// inside u:GetNATRSIPStatusResponse are not properly
			// namespaced.
			"POST /control?WANIPConnection": `
				 <s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
				 <s:Body>
				 <u:GetNATRSIPStatusResponse xmlns:u="urn:schemas-upnp-org:service:WANIPConnection:1">
				 <NewRSIPAvailable>0</NewRSIPAvailable>
				 <NewNATEnabled>1</NewNATEnabled>
				 </u:GetNATRSIPStatusResponse>
				 </s:Body>
				 </s:Envelope>
			`,
		},
	}
	return dev
}

// httpu.Handler
func (dev *FakeIGD) ServeMessage(r *http.Request) {
	conn, err := net.Dial("udp4", r.RemoteAddr)
	if err != nil {
		fmt.Printf("reply Dial error: %v", err)
		return
	}
	defer conn.Close()
	io.WriteString(conn, dev.replaceListenAddr(dev.ssdpResp))
}

// http.Handler
func (dev *FakeIGD) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	soadAction := r.Header.Get("SOAPACTION")
	if "" != soadAction {
		index := strings.Index(soadAction, "#")
		action := soadAction[index+1 : len(soadAction)-1]
		switch action {
		case "GetExternalIPAddress":
			resp := newSOAPEnvelope()
			resp.Body = soapBody{}
			resp.Body.RawAction = []byte("<s:Body><NewExternalIPAddress>192.168.1.1</NewExternalIPAddress></s:Body>")
			respB, _ := xml.Marshal(resp)
			w.Write(respB)
			return
		}
	}
	if resp, ok := dev.httpResps[r.Method+" "+r.RequestURI]; ok {
		io.WriteString(w, dev.replaceListenAddr(resp))
	} else {
		w.WriteHeader(http.StatusNotFound)
	}
}

func (dev *FakeIGD) replaceListenAddr(resp string) string {
	return strings.Replace(resp, "{{listenAddr}}", dev.listener.Addr().String(), -1)
}

func (dev *FakeIGD) Listen() (err error) {
	if dev.listener, err = net.Listen("tcp", "127.0.0.1:0"); err != nil {
		return err
	}
	laddr := &net.UDPAddr{IP: net.ParseIP("239.255.255.250"), Port: 1900}
	if dev.mcastListener, err = net.ListenMulticastUDP("udp", nil, laddr); err != nil {
		dev.listener.Close()
		return err
	}
	return nil
}

func (dev *FakeIGD) Serve() {
	go httpu.Serve(dev.mcastListener, dev)
	go http.Serve(dev.listener, dev)
}

func (dev *FakeIGD) Close() {
	dev.mcastListener.Close()
	dev.listener.Close()
}

// newSOAPAction creates a soapEnvelope with the given action and arguments.
func newSOAPEnvelope() *soapEnvelope {
	return &soapEnvelope{
		EncodingStyle: "http://schemas.xmlsoap.org/soap/encoding/",
	}
}

type soapEnvelope struct {
	XMLName       xml.Name `xml:"http://schemas.xmlsoap.org/soap/envelope/ Envelope"`
	EncodingStyle string   `xml:"http://schemas.xmlsoap.org/soap/envelope/ encodingStyle,attr"`
	Body          soapBody `xml:"http://schemas.xmlsoap.org/soap/envelope/ Body"`
}

type soapBody struct {
	Fault     *SOAPFaultError `xml:"Fault"`
	RawAction []byte          `xml:",innerxml"`
}

// SOAPFaultError implements error, and contains SOAP fault information.
type SOAPFaultError struct {
	FaultCode   string `xml:"faultcode"`
	FaultString string `xml:"faultstring"`
	Detail      string `xml:"detail"`
}

func (err *SOAPFaultError) Error() string {
	return fmt.Sprintf("SOAP fault: %s", err.FaultString)
}
