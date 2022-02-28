package main

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p/p2p/protocol/identify"
)

type daemon struct {
	host      host.Host
	idService identify.IDService
	// dht          kademlia
	// dhtMessenger *dhtpb.ProtocolMessenger
}

type ephemeralHost struct {
	host      host.Host
	idService identify.IDService
	// dht          kademlia
	// dhtMessenger *dhtpb.ProtocolMessenger
}

type IdentifyOutput struct {
	ParseAddressError  string   `json:"parse_address_error,omitempty"`
	ConnectToPeerError string   `json:"connect_to_peer_error,omitempty"`
	IdentifyPeerError  string   `json:"identify_peer_error,omitempty"`
	Protocols          []string `json:"protocols,omitempty"`
	Addresses          []string `json:"addresses,omitempty"`
}

func newEphemeralHost() ephemeralHost {
	// h, err := libp2p.New(libp2p.ConnectionGater(&privateAddrFilterConnectionGater{}))
	h, err := libp2p.New() // TODO: in original we forbid private addr, why?
	if err != nil {
		panic(err) // TODO: handle better
	}

	id, err := identify.NewIDService(h, identify.UserAgent("ipfs-check"))
	if err != nil {
		panic(err) // TODO: handle better
	}

	return ephemeralHost{host: h, idService: id}
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

	ctx := context.Background()

	e := newEphemeralHost()
	defer e.host.Close()
	defer e.idService.Close()

	dialCtx, dialCancel := context.WithTimeout(ctx, 30*time.Second)
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

	// TODO: I would like to get the address seen by the other peer. It should be in the identify protocol payload.

	return out, nil
}
