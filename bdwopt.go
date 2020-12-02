package pubsub

import (
	"fmt"

	pb "github.com/bdware/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p-core/protocol"
)

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

// WithCustomProtocols changes the protocols of pubsub.
func WithCustomProtocols(protos []protocol.ID) Option {
	return func(p *PubSub) error {
		if len(protos) == 0 {
			return fmt.Errorf("unexpected empty protos")
		}
		switch rt := p.rt.(type) {
		case *RandomSubRouter:
			rt.protocols = protos
		case *FloodSubRouter:
			rt.protocols = protos
		case *GossipSubRouter:
			rt.customProtocols = protos
		default:
			return fmt.Errorf("unknown router type")
		}

		return nil
	}
}
