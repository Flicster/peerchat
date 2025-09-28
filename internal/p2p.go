package internal

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"sync"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p"
	connmgr "github.com/libp2p/go-libp2p-connmgr"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/routing"
	discovery "github.com/libp2p/go-libp2p-discovery"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	tls "github.com/libp2p/go-libp2p-tls"
	yamux "github.com/libp2p/go-libp2p-yamux"
	"github.com/libp2p/go-tcp-transport"
	"github.com/mr-tron/base58/base58"
	"github.com/multiformats/go-multiaddr"
	"github.com/multiformats/go-multihash"
	"github.com/sirupsen/logrus"
)

const service = "peerchat"

type P2P struct {
	Ctx       context.Context
	Host      host.Host
	KadDHT    *dht.IpfsDHT
	Discovery *discovery.RoutingDiscovery
	PubSub    *pubsub.PubSub
}

/*
NewP2P function generates and returns a P2P object.

Constructs a libp2p host with TLS encrypted secure transportation that works over a TCP
transport connection using a Yamux Stream Multiplexer and uses UPnP for the NAT traversal.

A Kademlia DHT is then bootstrapped on this host using the default peers offered by libp2p
and a Peer Discovery service is created from this Kademlia DHT. The PubSub handler is then
created on the host using the peer discovery service created prior.
*/
func NewP2P() *P2P {
	ctx := context.Background()

	nodehost, kaddht := setupHost(ctx)
	logrus.Debugln("Created the P2P Host and the Kademlia DHT.")

	bootstrapDHT(ctx, nodehost, kaddht)
	logrus.Debugln("Bootstrapped the Kademlia DHT and Connected to Bootstrap Peers")

	routingdiscovery := discovery.NewRoutingDiscovery(kaddht)
	logrus.Debugln("Created the Peer Discovery Service.")

	pubsubhandler := setupPubSub(ctx, nodehost, routingdiscovery)
	logrus.Debugln("Created the PubSub Handler.")

	return &P2P{
		Ctx:       ctx,
		Host:      nodehost,
		KadDHT:    kaddht,
		Discovery: routingdiscovery,
		PubSub:    pubsubhandler,
	}
}

// AdvertiseConnect connects to service peers.
// This method uses the Advertise() functionality of the Peer Discovery Service
// to advertise the service and then disovers all peers advertising the same.
// The peer discovery is handled by a go-routine that will read from a channel
// of peer address information until the peer channel closes
func (p2p *P2P) AdvertiseConnect() {
	ttl, err := p2p.Discovery.Advertise(p2p.Ctx, service)
	logrus.Debugln("Advertised the PeerChat Service.")
	time.Sleep(time.Second * 5)
	logrus.Debugf("Service Time-to-Live is %s", ttl)

	peerchan, err := p2p.Discovery.FindPeers(p2p.Ctx, service)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Fatalln("P2P Peer Discovery Failed!")
	}
	logrus.Traceln("Discovered PeerChat Service Peers.")

	go handlePeerDiscovery(p2p.Host, peerchan)
	logrus.Traceln("Started Peer Connection Handler.")
}

// AnnounceConnect connects to service peers.
// This method uses the Provide() functionality of the Kademlia DHT directly to announce
// the ability to provide the service and then disovers all peers that provide the same.
// The peer discovery is handled by a go-routine that will read from a channel
// of peer address information until the peer channel closes
func (p2p *P2P) AnnounceConnect() {
	cidvalue := generateCID(service)
	logrus.Traceln("Generated the Service CID.")

	err := p2p.KadDHT.Provide(p2p.Ctx, cidvalue, true)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Fatalln("Failed to Announce Service CID!")
	}
	logrus.Debugln("Announced the PeerChat Service.")
	time.Sleep(time.Second * 5)

	peerchan := p2p.KadDHT.FindProvidersAsync(p2p.Ctx, cidvalue, 0)
	logrus.Traceln("Discovered PeerChat Service Peers.")

	go handlePeerDiscovery(p2p.Host, peerchan)
	logrus.Debugln("Started Peer Connection Handler.")
}

// setupHost generates the p2p configuration options and creates a
// libp2p host object for the given context. The created host is returned
func setupHost(ctx context.Context) (host.Host, *dht.IpfsDHT) {
	prvkey, _, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, rand.Reader)
	identity := libp2p.Identity(prvkey)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Fatalln("Failed to Generate P2P Identity Configuration!")
	}

	logrus.Traceln("Generated P2P Identity Configuration.")

	tlstransport, err := tls.New(prvkey)
	security := libp2p.Security(tls.ID, tlstransport)
	transport := libp2p.Transport(tcp.NewTCPTransport)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Fatalln("Failed to Generate P2P Security and Transport Configurations!")
	}

	logrus.Traceln("Generated P2P Security and Transport Configurations.")

	muladdr, err := multiaddr.NewMultiaddr("/ip4/0.0.0.0/tcp/0")
	listen := libp2p.ListenAddrs(muladdr)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Fatalln("Failed to Generate P2P Address Listener Configuration!")
	}

	logrus.Traceln("Generated P2P Address Listener Configuration.")

	muxer := libp2p.Muxer("/yamux/1.0.0", yamux.DefaultTransport)
	conn := libp2p.ConnectionManager(connmgr.NewConnManager(100, 400, time.Minute))

	logrus.Traceln("Generated P2P Stream Multiplexer, Connection Manager Configurations.")

	nat := libp2p.NATPortMap()
	relay := libp2p.EnableAutoRelay()

	logrus.Traceln("Generated P2P NAT Traversal and Relay Configurations.")

	var kaddht *dht.IpfsDHT
	routing := libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
		kaddht = setupKadDHT(ctx, h)
		return kaddht, err
	})

	logrus.Traceln("Generated P2P Routing Configurations.")

	opts := libp2p.ChainOptions(identity, listen, security, transport, muxer, conn, nat, routing, relay)

	libhost, err := libp2p.New(ctx, opts)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Fatalln("Failed to Create the P2P Host!")
	}

	return libhost, kaddht
}

// setupKadDHT generates a Kademlia DHT object and returns it
func setupKadDHT(ctx context.Context, nodehost host.Host) *dht.IpfsDHT {
	dhtmode := dht.Mode(dht.ModeServer)
	bootstrappeers := dht.GetDefaultBootstrapPeerAddrInfos()
	dhtpeers := dht.BootstrapPeers(bootstrappeers...)

	logrus.Traceln("Generated DHT Configuration.")

	kaddht, err := dht.New(ctx, nodehost, dhtmode, dhtpeers)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Fatalln("Failed to Create the Kademlia DHT!")
	}

	return kaddht
}

// setupPubSub generates a PubSub Handler object and returns it
// Requires a node host and a routing discovery service.
func setupPubSub(ctx context.Context, nodehost host.Host, routingdiscovery *discovery.RoutingDiscovery) *pubsub.PubSub {
	pubsubhandler, err := pubsub.NewGossipSub(ctx, nodehost, pubsub.WithDiscovery(routingdiscovery))
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err.Error(),
			"type":  "GossipSub",
		}).Fatalln("PubSub Handler Creation Failed!")
	}

	return pubsubhandler
}

// bootstrapDHT bootstraps a given Kademlia DHT to satisfy the IPFS router
// interface and connects to all the bootstrap peers provided by libp2p
func bootstrapDHT(ctx context.Context, nodehost host.Host, kaddht *dht.IpfsDHT) {
	if err := kaddht.Bootstrap(ctx); err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Fatalln("Failed to Bootstrap the Kademlia!")
	}

	logrus.Traceln("Set the Kademlia DHT into Bootstrap Mode.")

	var wg sync.WaitGroup
	var connectedbootpeers int
	var totalbootpeers int

	for _, peeraddr := range dht.DefaultBootstrapPeers {
		peerinfo, _ := peer.AddrInfoFromP2pAddr(peeraddr)

		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := nodehost.Connect(ctx, *peerinfo); err != nil {
				totalbootpeers++
			} else {
				connectedbootpeers++
				totalbootpeers++
			}
		}()
	}
	wg.Wait()

	logrus.Debugf("Connected to %d out of %d Bootstrap Peers.", connectedbootpeers, totalbootpeers)
}

// handlePeerDiscovery connects the given host to all peers recieved from a
// channel of peer address information. Meant to be started as a go routine.
func handlePeerDiscovery(nodehost host.Host, peerchan <-chan peer.AddrInfo) {
	for peer := range peerchan {
		if peer.ID == nodehost.ID() {
			continue
		}
		nodehost.Connect(context.Background(), peer)
	}
}

// generateCID generates a CID object for a given string and returns it.
// Uses SHA256 to hash the string and generate a multihash from it.
// The mulithash is then base58 encoded and then used to create the CID
func generateCID(namestring string) cid.Cid {
	hash := sha256.Sum256([]byte(namestring))
	finalhash := append([]byte{0x12, 0x20}, hash[:]...)
	b58string := base58.Encode(finalhash)

	mulhash, err := multihash.FromB58String(string(b58string))
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Fatalln("Failed to Generate Service CID!")
	}

	cidvalue := cid.NewCidV1(12, mulhash)
	return cidvalue
}
