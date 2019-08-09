package config

// ServiceFlag identifies services supported by a bitcoin peer.
type ServiceFlag uint64

const (
	// SFNodeTX is a flag used to indicate a peer is a supports broadcasting tx.
	SFNodeTX ServiceFlag = iota

	// SFNodeBlockBroadCast is a flag used to indicate a peer supports broadcasting block.
	SFNodeBlockBroadCast

	// SFNodeBlockBraodSyncer is a flag used to indicate a peer supports synchronizing block
	SFNodeBlockSyncer

	// SFNodeBlockBraodSyncer is a test flag used to test p2p network
	SFNodeBroadCastTest
)

// P2PConfig configuration of the p2p network.
type P2PConfig struct {
	AddrBookFilePath string      // address book file path
	ListenAddress    string      // server listen address
	MaxConnOutBound  int         // max connection out bound
	MaxConnInBound   int         // max connection in bound
	PersistentPeers  string      // persistent peers
	DebugServer      string      // p2p test debug server address
	DebugP2P         bool        // p2p debug flag
	DebugAddr        string      //debug address
	NAT              string      //NAT port mapping mechanism(none|upnp)
	SeedMode         bool        // whether run as dns seed(default false)
	DisableDNSSeed   bool        //Disable DNS seeding for peers
	DNSSeeds         string      //list of DNS seeds for the network that are used as one method to discover peers
	Service          ServiceFlag // service supported by this peer.
	WorkerPoolSize   int         // worker pool size
}
