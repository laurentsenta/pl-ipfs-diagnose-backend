package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	kadDHT "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/p2p/protocol/identify"
	"github.com/libp2p/go-libp2p/p2p/protocol/ping"
)

type daemon struct {
	host      host.Host
	idService identify.IDService
	// dht          kademlia
	// dhtMessenger *dhtpb.ProtocolMessenger
}

type ephemeralHost struct {
	// TODO: figure out what happens with ref / values x struct / interface
	host        host.Host
	idService   identify.IDService
	pingService *ping.PingService
	dht         *kadDHT.IpfsDHT
	// dhtMessenger *dhtpb.ProtocolMessenger
}

type IdentifyOutput struct {
	ParseAddressError  string   `json:"parse_address_error,omitempty"`
	ConnectToPeerError string   `json:"connect_to_peer_error,omitempty"`
	IdentifyPeerError  string   `json:"identify_peer_error,omitempty"`
	PingError          string   `json:"ping_error,omitempty"`
	PingDurationMS     int64    `json:"ping_duration_ms,omitempty"`
	Protocols          []string `json:"protocols,omitempty"`
	Addresses          []string `json:"addresses,omitempty"`
}

func newEphemeralHost(ctx context.Context) ephemeralHost {
	// h, err := libp2p.New(libp2p.ConnectionGater(&privateAddrFilterConnectionGater{}))
	h, err := libp2p.New() // TODO: in original we forbid private addr, why?
	if err != nil {
		panic(err) // TODO: handle better
	}

	id, err := identify.NewIDService(h, identify.UserAgent("ipfs-check"))
	if err != nil {
		panic(err) // TODO: handle better
	}

	p := ping.NewPingService(h)

	// There are 3 different new function it's hard to tell which one to use.
	// dht, err := kadDHT.New(ctx, h, kadDHT.BootstrapPeers(kadDHT.GetDefaultBootstrapPeerAddrInfos()...))

	// new dht client takes a datastore and ignores bootstrap, why is the interface different from New? Does it means it doesn't
	// traverse the DHT at all (no bootstrap nodes)?
	// dht := kadDHT.NewDHTClient(ctx, h, datastore.NewMapDatastore())

	// Apparently this is pretty much identical to the DHTClient call.
	dht, err := kadDHT.New(ctx, h, kadDHT.Mode(kadDHT.ModeClient), kadDHT.BootstrapPeers(kadDHT.GetDefaultBootstrapPeerAddrInfos()...))
	if err != nil {
		panic(err) // TODO: handle better
	}

	// NOTE: Bootstrap tells the DHT to get into a bootstrapped state satisfying the IpfsRouter interface.
	// I have no idea what that means, I just want to make sure the node connects at least to my bootstraping peers
	err = dht.Bootstrap(ctx)
	if err != nil {
		panic(err) // TODO: handle better
	}

	log.Println("default bootstrap:", kadDHT.GetDefaultBootstrapPeerAddrInfos())
	log.Println("current peers:", h.Network().Peers())

	return ephemeralHost{host: h, idService: id, pingService: p, dht: dht}
}

func (d *daemon) runIdentify(writer http.ResponseWriter, uristr string) (IdentifyOutput, error) {
	out := IdentifyOutput{}

	u, err := url.ParseRequestURI(uristr)
	if err != nil {
		return out, err
	}

	maddr := u.Query().Get("addr")

	if maddr == "" {
		return out, errors.New("missing argument: addr")
	}

	ai, err := peer.AddrInfoFromString(maddr)

	if err != nil {
		out.ParseAddressError = err.Error()
		return out, nil
	}

	// TODO: probably need a different ctx for the active request?
	ctx := context.Background()

	e := newEphemeralHost(ctx)
	defer e.host.Close()
	defer e.idService.Close()

	dialCtx, dialCancel := context.WithTimeout(ctx, 15*time.Second)
	defer dialCancel()

	// Note: I don't understand this API, I have to connect with a peer addr
	// before having the ability to call the dialPeer and get the conn for identify?

	// In the examples it's also quite convoluted,
	// I have to add the node to the peer store, before having
	// the ability to call connect.

	// Similarly, to get the identify payload you pretty much have to reconstruct
	// the data, I couldn't find the current snapshot of the identify for a given peer.
	e.host.Peerstore().AddAddrs(ai.ID, ai.Addrs, time.Hour)
	err = e.host.Connect(ctx, *ai)

	if err != nil {
		out.ConnectToPeerError = err.Error()
		return out, nil
	}

	conn, err := e.host.Network().DialPeer(dialCtx, ai.ID)

	if err != nil {
		out.ConnectToPeerError = err.Error()
		return out, nil
	}

	// ping node
	ping := e.pingService.Ping(dialCtx, ai.ID)
	result := <-ping

	if result.Error != nil {
		out.PingError = result.Error.Error()
		return out, nil
	}
	out.PingDurationMS = result.RTT.Milliseconds()

	// identify node
	identifyC := d.idService.IdentifyWait(conn)

	select {
	case <-dialCtx.Done():
		out.IdentifyPeerError = "timeout when trying to identify the peer"
		return out, nil
	case <-identifyC:
		// done
	}

	protocols, err := e.host.Peerstore().GetProtocols(ai.ID)

	if err != nil {
		out.IdentifyPeerError = err.Error()
		return out, nil
	}

	out.Protocols = protocols

	addresses := e.host.Peerstore().Addrs(ai.ID)

	strAddresses := make([]string, len(addresses))

	for i, a := range addresses {
		strAddresses[i] = a.String()
	}

	out.Addresses = strAddresses

	// TODO: I would like to get the address seen by the other peer.
	// It should be in the identify protocol payload.

	return out, nil
}

type FindContentOutput struct {
	ParseCIDError      string   `json:"parse_cid_error,omitempty"`
	FindProvidersError string   `json:"find_providers_error,omitempty"`
	Providers          []string `json:"providers,omitempty"`
	ProvidersError     string   `json:"providers_error,omitempty"`
	// PingError          string   `json:"ping_error,omitempty"`
	// PingDurationMS     int64    `json:"ping_duration_ms,omitempty"`
	// Addresses          []string `json:"addresses,omitempty"`
}

func (d *daemon) runFindContent(writer http.ResponseWriter, uristr string) (FindContentOutput, error) {
	out := FindContentOutput{}

	u, err := url.ParseRequestURI(uristr)
	if err != nil {
		return out, err
	}

	cidstr := u.Query().Get("cid")

	if cidstr == "" {
		return out, errors.New("missing argument: cid")
	}

	c, err := cid.Decode(cidstr)

	if err != nil {
		out.ParseCIDError = err.Error()
		return out, nil
	}

	ctx := context.Background()

	e := newEphemeralHost(ctx)
	defer e.host.Close()
	defer e.idService.Close()

	dialCtx, dialCancel := context.WithTimeout(ctx, 15*time.Second)
	defer dialCancel()

	providers, err := e.dht.FindProviders(dialCtx, c)

	log.Println(e.host.Network().Peers())

	if err != nil {
		out.FindProvidersError = err.Error()
		return out, nil
	}

	strProviders := make([]string, len(providers))

	for i, a := range providers {
		strProviders[i] = a.String()
	}

	if len(providers) == 0 {
		out.ProvidersError = "no providers found"
	}

	out.Providers = strProviders

	return out, nil
}
