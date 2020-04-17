package pubsub

import (
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-core/protocol"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
)

func TestRandomSubDGenerator(t *testing.T) {
	// subNum should be larger than defaultRandomSubD
	subNum := 10
	subhs := getNetHosts(t, context.Background(), subNum)
	pubh := getNetHosts(t, context.Background(), 1)[0]
	var subs []*PubSub
	for _, subh := range subhs {
		sub, err := NewRandomSub(context.Background(), subh)
		assert.NoError(t, err)
		subs = append(subs, sub)
	}
	testgen := func(msg *pb.Message) int {
		// change randomSubD
		return subNum
	}
	pub, err := NewRandomSub(context.Background(), pubh, WithRandomSubDGenerator(testgen))
	assert.NoError(t, err)

	// subscribe
	topic := "test"
	var subchs []*Subscription
	for _, sub := range subs {
		subch, err := sub.Subscribe(topic)
		assert.NoError(t, err)
		subchs = append(subchs, subch)
	}
	for _, subh := range subhs {
		connect(t, pubh, subh)
	}

	// check message
	checkMessageRouting(t, topic, []*PubSub{pub}, subchs)

}

func TestRandomSubDWithDifferentProtocols(t *testing.T) {
	subhs := getNetHosts(t, context.Background(), 2)
	pubh := getNetHosts(t, context.Background(), 1)[0]

	pub, err := NewRandomSub(
		context.Background(),
		pubh,
		WithCustomProtocols([]protocol.ID{
			protocol.ID("foobar"),
		}),
	)
	assert.NoError(t, err)

	var subs []*PubSub
	var subprotos = []protocol.ID{
		protocol.ID("foobar"),
		protocol.ID("another"),
	}
	for i, subh := range subhs {
		sub, err := NewRandomSub(
			context.Background(),
			subh,
			WithCustomProtocols([]protocol.ID{
				subprotos[i],
			}),
		)
		assert.NoError(t, err)
		subs = append(subs, sub)
	}

	// subscribe
	topic := "test"
	var subchs []*Subscription
	for _, sub := range subs {
		subch, err := sub.Subscribe(topic)
		assert.NoError(t, err)
		subchs = append(subchs, subch)
	}
	for _, subh := range subhs {
		connect(t, pubh, subh)
	}

	// assert the different protocol pubsub receives nothing
	tctx, _ := context.WithTimeout(context.Background(), 100*time.Millisecond)
	msg, err := subchs[1].Next(tctx)
	assert.Nil(t, msg)
	assert.NotNil(t, err)
	// check message
	checkMessageRouting(t, topic, []*PubSub{pub}, subchs[:1])

}
