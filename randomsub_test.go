package pubsub

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-core/protocol"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/stretchr/testify/assert"

	"github.com/libp2p/go-libp2p-core/host"
)

func getRandomsub(ctx context.Context, h host.Host, size int, opts ...Option) *PubSub {
	ps, err := NewRandomSub(ctx, h, size, opts...)
	if err != nil {
		panic(err)
	}
	return ps
}

func getRandomsubs(ctx context.Context, hs []host.Host, size int, opts ...Option) []*PubSub {
	var psubs []*PubSub
	for _, h := range hs {
		psubs = append(psubs, getRandomsub(ctx, h, size, opts...))
	}
	return psubs
}

func tryReceive(sub *Subscription) *Message {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	m, err := sub.Next(ctx)
	if err != nil {
		return nil
	} else {
		return m
	}
}

func TestRandomsubSmall(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hosts := getNetHosts(t, ctx, 10)
	psubs := getRandomsubs(ctx, hosts, 10)

	connectAll(t, hosts)

	var subs []*Subscription
	for _, ps := range psubs {
		sub, err := ps.Subscribe("test")
		if err != nil {
			t.Fatal(err)
		}
		subs = append(subs, sub)
	}

	time.Sleep(time.Second)

	count := 0
	for i := 0; i < 10; i++ {
		msg := []byte(fmt.Sprintf("message %d", i))
		psubs[i].Publish("test", msg)

		for _, sub := range subs {
			if tryReceive(sub) != nil {
				count++
			}
		}
	}

	if count < 7*len(hosts) {
		t.Fatalf("received too few messages; expected at least %d but got %d", 9*len(hosts), count)
	}
}

func TestRandomsubBig(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hosts := getNetHosts(t, ctx, 50)
	psubs := getRandomsubs(ctx, hosts, 50)

	connectSome(t, hosts, 12)

	var subs []*Subscription
	for _, ps := range psubs {
		sub, err := ps.Subscribe("test")
		if err != nil {
			t.Fatal(err)
		}
		subs = append(subs, sub)
	}

	time.Sleep(time.Second)

	count := 0
	for i := 0; i < 10; i++ {
		msg := []byte(fmt.Sprintf("message %d", i))
		psubs[i].Publish("test", msg)

		for _, sub := range subs {
			if tryReceive(sub) != nil {
				count++
			}
		}
	}

	if count < 7*len(hosts) {
		t.Fatalf("received too few messages; expected at least %d but got %d", 9*len(hosts), count)
	}
}

func TestRandomsubMixed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hosts := getNetHosts(t, ctx, 40)
	fsubs := getPubsubs(ctx, hosts[:10])
	rsubs := getRandomsubs(ctx, hosts[10:], 30)
	psubs := append(fsubs, rsubs...)

	connectSome(t, hosts, 12)

	var subs []*Subscription
	for _, ps := range psubs {
		sub, err := ps.Subscribe("test")
		if err != nil {
			t.Fatal(err)
		}
		subs = append(subs, sub)
	}

	time.Sleep(time.Second)

	count := 0
	for i := 0; i < 10; i++ {
		msg := []byte(fmt.Sprintf("message %d", i))
		psubs[i].Publish("test", msg)

		for _, sub := range subs {
			if tryReceive(sub) != nil {
				count++
			}
		}
	}

	if count < 7*len(hosts) {
		t.Fatalf("received too few messages; expected at least %d but got %d", 9*len(hosts), count)
	}
}

func TestRandomsubEnoughPeers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hosts := getNetHosts(t, ctx, 40)
	fsubs := getPubsubs(ctx, hosts[:10])
	rsubs := getRandomsubs(ctx, hosts[10:], 30)
	psubs := append(fsubs, rsubs...)

	connectSome(t, hosts, 12)

	var subs []*Subscription
	for _, ps := range psubs {
		sub, err := ps.Subscribe("test")
		if err != nil {
			t.Fatal(err)
		}
		subs = append(subs, sub)
	}

	time.Sleep(time.Second)

	res := make(chan bool, 1)
	rsubs[0].eval <- func() {
		rs := rsubs[0].rt.(*RandomSubRouter)
		res <- rs.EnoughPeers("test", 0)
	}

	enough := <-res
	if !enough {
		t.Fatal("expected enough peers")
	}

	rsubs[0].eval <- func() {
		rs := rsubs[0].rt.(*RandomSubRouter)
		res <- rs.EnoughPeers("test", 100)
	}

	enough = <-res
	if !enough {
		t.Fatal("expected enough peers")
	}
}

var (
	defaultSize = 10
)

func TestRandomSubDGenerator(t *testing.T) {
	// subNum should be larger than defaultRandomSubD
	subNum := 10
	subhs := getNetHosts(t, context.Background(), subNum)
	pubh := getNetHosts(t, context.Background(), 1)[0]
	var subs []*PubSub
	for _, subh := range subhs {
		sub, err := NewRandomSub(context.Background(), subh, defaultSize)
		assert.NoError(t, err)
		subs = append(subs, sub)
	}
	testgen := func(msg *pb.Message) int {
		// change randomSubD
		return subNum
	}
	pub, err := NewRandomSub(
		context.Background(),
		pubh,
		defaultSize,
		WithRandomSubDGenerator(testgen))
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
		defaultSize,
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
			defaultSize,
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
