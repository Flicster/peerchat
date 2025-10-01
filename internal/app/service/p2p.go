package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	discovery "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

const serviceName = "peerchat"

type P2P struct {
	Ctx       context.Context
	Host      host.Host
	Discovery *discovery.RoutingDiscovery
	PubSub    *pubsub.PubSub
}

func NewP2P() (*P2P, error) {
	ctx := context.Background()

	priv, _, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate identity key: %w", err)
	}

	cm, err := connmgr.NewConnManager(100, 400, connmgr.WithGracePeriod(time.Minute))
	if err != nil {
		return nil, fmt.Errorf("create conn manager: %w", err)
	}

	h, err := libp2p.New(
		libp2p.Identity(priv),
		libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/0"),
		libp2p.ConnectionManager(cm),
		libp2p.NATPortMap(),
		libp2p.EnableRelay(),
		libp2p.EnableHolePunching(),
	)
	if err != nil {
		return nil, fmt.Errorf("create p2p: %w", err)
	}
	logrus.Debugf("created host: %s", h.ID().String())

	kaddht, err := dht.New(ctx, h, dht.Mode(dht.ModeServer))
	if err != nil {
		_ = h.Close()
		return nil, fmt.Errorf("create dht: %w", err)
	}
	if err = kaddht.Bootstrap(ctx); err != nil {
		return nil, fmt.Errorf("bootstrap kaddht: %w", err)
	}
	for _, addr := range dht.DefaultBootstrapPeers {
		pi, _ := peer.AddrInfoFromP2pAddr(addr)
		if err = h.Connect(ctx, *pi); err != nil {
			logrus.WithError(err).Warn("failed to connect bootstrap peer")
		}
	}

	routingDiscovery := discovery.NewRoutingDiscovery(kaddht)

	ps, err := pubsub.NewGossipSub(ctx, h, pubsub.WithDiscovery(routingDiscovery))
	if err != nil {
		_ = kaddht.Close()
		_ = h.Close()
		return nil, fmt.Errorf("create gossipsub: %w", err)
	}

	return &P2P{
		Ctx:       ctx,
		Host:      h,
		Discovery: routingDiscovery,
		PubSub:    ps,
	}, nil
}

func (p *P2P) AdvertiseConnect() error {
	ttl, err := p.Discovery.Advertise(p.Ctx, serviceName)
	if err != nil {
		return fmt.Errorf("discovery advertise: %w", err)
	}
	logrus.Debugf("advertised service %q (ttl=%s)", serviceName, ttl)

	peerCh, err := p.Discovery.FindPeers(p.Ctx, serviceName)
	if err != nil {
		return fmt.Errorf("find peers: %w", err)
	}
	go p.handlePeerDiscovery(peerCh)
	return nil
}

func (p *P2P) GetPeerID() peer.ID {
	if p == nil || p.Host == nil {
		return ""
	}
	return p.Host.ID()
}

func (p *P2P) handlePeerDiscovery(peerCh <-chan peer.AddrInfo) error {
	g, ctx := errgroup.WithContext(p.Ctx)
	g.SetLimit(5)

	for pi := range peerCh {
		if pi.ID == p.Host.ID() || len(pi.Addrs) == 0 {
			continue
		}

		peerInfo := pi

		g.Go(func() error {
			connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			if err := p.Host.Connect(connectCtx, peerInfo); err != nil {
				logrus.WithError(err).WithFields(logrus.Fields{
					"peer": peerInfo.ID.String(),
				}).Debug("failed to connect peer")
				return nil
			}

			logrus.WithField("peer", peerInfo.ID.String()).Debug("connected peer")
			return nil
		})
	}
	return g.Wait()
}
