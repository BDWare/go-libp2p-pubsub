package pubsub

import (
	"testing"

	pb "bdware.org/libp2p/go-libp2p-pubsub/pb"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
)

func TestRandomSubDGenerator(t *testing.T) {
	// subNum should be larger than defaultRandomSubD
	subNum := 10
	subhs := getNetHosts(t, context.Background(), subNum)
	pubh := getNetHosts(t, context.Background(), 1)[0]
	var subs []*PubSub
	var pub *PubSub
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
