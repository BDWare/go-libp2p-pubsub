// Copyright for portions of this fork are held by [Jeromy Johnson, 2016] as
// part of the original go-libp2p-pubsub project. All other copyright for
//  this fork are held by [The BDWare Authors, 2020]. All rights reserved.

package pubsub

import (
	"context"
	"fmt"
	"math"

	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
)

// RandomSubID is the default protocol ID used by randomSub
const (
	RandomSubID = protocol.ID("/randomsub/1.0.0")
)

// RandomSubD is the default number of peers to be forwarded.
var (
	RandomSubD = 6
)

// NewRandomSub returns a new PubSub object using RandomSubRouter as the router.
func NewRandomSub(ctx context.Context, h host.Host, size int, opts ...Option) (*PubSub, error) {
	rt := &RandomSubRouter{
		peers:     make(map[peer.ID]protocol.ID),
		gen:       DefaultRandomSubDGenerator,
		protocols: []protocol.ID{RandomSubID, FloodSubID},
		size:      size,
	}
	return NewPubSub(ctx, h, rt, opts...)
}

// RandomSubRouter is a router that implements a random propagation strategy.
// For each message, it selects the square root of the network size peers, with a min of RandomSubD,
// and forwards the message to them.
type RandomSubRouter struct {
	p         *PubSub
	peers     map[peer.ID]protocol.ID
	tracer    *pubsubTracer
	gen       RandomSubDGenerator
	protocols []protocol.ID
	size      int
}

func (rs *RandomSubRouter) Protocols() []protocol.ID {
	return rs.protocols
}

func (rs *RandomSubRouter) Attach(p *PubSub) {
	rs.p = p
	rs.tracer = p.tracer
}

func (rs *RandomSubRouter) AddPeer(p peer.ID, proto protocol.ID) {
	rs.tracer.AddPeer(p, proto)
	rs.peers[p] = proto
}

func (rs *RandomSubRouter) RemovePeer(p peer.ID) {
	rs.tracer.RemovePeer(p)
	delete(rs.peers, p)
}

func (rs *RandomSubRouter) EnoughPeers(topic string, suggested int) bool {
	// check all peers in the topic
	tmap, ok := rs.p.topics[topic]
	if !ok {
		return false
	}

	fsPeers := 0
	rsPeers := 0

	// count floodsub and randomsub peers
	for p := range tmap {
		switch rs.peers[p] {
		case FloodSubID:
			fsPeers++
		case RandomSubID:
			rsPeers++
		}
	}

	if suggested == 0 {
		suggested = RandomSubD
	}

	if fsPeers+rsPeers >= suggested {
		return true
	}

	if rsPeers >= RandomSubD {
		return true
	}

	return false
}

func (rs *RandomSubRouter) AcceptFrom(peer.ID) bool {
	return true
}

func (rs *RandomSubRouter) HandleRPC(rpc *RPC) {}

func (rs *RandomSubRouter) Publish(msg *Message) {
	from := msg.ReceivedFrom

	tosend := make(map[peer.ID]struct{})
	rspeers := make(map[peer.ID]struct{})
	src := peer.ID(msg.GetFrom())

	for _, topic := range msg.GetTopicIDs() {
		tmap, ok := rs.p.topics[topic]
		if !ok {
			continue
		}

		for p := range tmap {
			if p == from || p == src {
				continue
			}

			if rs.peers[p] == FloodSubID {
				tosend[p] = struct{}{}
			} else {
				rspeers[p] = struct{}{}
			}
		}
	}

	// get randomSubD for each massage
	randomSubD := rs.gen(msg.Message)

	if len(rspeers) > randomSubD {
		target := randomSubD
		sqrt := int(math.Ceil(math.Sqrt(float64(rs.size))))
		if sqrt > target {
			target = sqrt
		}
		if target > len(rspeers) {
			target = len(rspeers)
		}
		xpeers := peerMapToList(rspeers)
		shufflePeers(xpeers)
		xpeers = xpeers[:randomSubD]
		for _, p := range xpeers {
			tosend[p] = struct{}{}
		}
	} else {
		for p := range rspeers {
			tosend[p] = struct{}{}
		}
	}

	out := rpcWithMessages(msg.Message)
	sentPeers := make([]peer.ID, 0, len(tosend))
	for p := range tosend {
		mch, ok := rs.p.peers[p]
		if !ok {
			continue
		}

		select {
		case mch <- out:
			rs.tracer.SendRPC(out, p)
			sentPeers = append(sentPeers, p)
		default:
			log.Infof("dropping message to peer %s: queue full", p)
			rs.tracer.DropRPC(out, p)
		}
	}

	rs.tracer.SendMessageDone(msg, sentPeers)
}

func (rs *RandomSubRouter) Join(topic string) {
	rs.tracer.Join(topic)
}

func (rs *RandomSubRouter) Leave(topic string) {
	rs.tracer.Join(topic)
}

/* BDWare */

// RandomSubDGenerator is the function that controls how many peers to be forwarded.
type RandomSubDGenerator func(msg *pb.Message) int

// DefaultRandomSubDGenerator returns the default RandomSubD
func DefaultRandomSubDGenerator(msg *pb.Message) int {
	return RandomSubD
}

// WithRandomSubDGenerator is the option that changes randomSubDGenerator
func WithRandomSubDGenerator(gen RandomSubDGenerator) Option {
	return func(p *PubSub) error {
		rt, ok := p.rt.(*RandomSubRouter)
		// check rt's type
		if !ok {
			return fmt.Errorf("unexpected router type: need to be RandomSub")
		}
		if rt.gen == nil {
			return fmt.Errorf("unexpected nil generator")
		}
		// change rt's generator
		rt.gen = gen
		return nil
	}
}

// WithCustomProtocols changes the protocols of randomsub.
func WithCustomProtocols(protos []protocol.ID) Option {
	return func(p *PubSub) error {
		rt, ok := p.rt.(*RandomSubRouter)
		// check rt's type
		if !ok {
			return fmt.Errorf("unexpected router type: need to be RandomSub")
		}
		if len(protos) == 0 {
			return fmt.Errorf("unexpected empty protos")
		}
		// change rt's generator
		rt.protocols = protos
		return nil
	}
}
