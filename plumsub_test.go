package pubsub

import (
	"bytes"
	"context"
	"fmt"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/assert"
	"math/rand"
	"os"
	"testing"
	"time"

	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/host"
)

func getPlumsub(ctx context.Context, h host.Host, opts ...Option) *PubSub {
	logging.SetLogLevel("plumsub", "debug")
	ps, err := NewRoutablePlumSub(ctx, h, opts...)
	if err != nil {
		panic(err)
	}
	return ps
}

func getPlumsubs(ctx context.Context, hs []host.Host, opts ...Option) []*PubSub {
	var psubs []*PubSub
	for _, h := range hs {
		psubs = append(psubs, getPlumsub(ctx, h, opts...))
	}
	return psubs
}

func printConns(psubs []*PubSub) {
	for i := range psubs {
		peers := psubs[i].ListPeers("foobar")
		fmt.Printf("(%d, %s):\n", i, psubs[i].host.ID().ShortString())
		for j := range peers {
			fmt.Println(peers[j].ShortString())
		}
	}
}
func printRouters(psubs []*PubSub) {
	for i := range psubs {
		rt, ok := psubs[i].rt.(*PlumRouter)
		if !ok {
			continue
		}
		for _, srt := range rt.rts {
			srt.print(os.Stdout)
		}
	}
}

func TestBasicPlumsub(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hosts := getNetHosts(t, ctx, 2)

	psubs := getPlumsubs(ctx, hosts)

	var msgs []*Subscription
	for _, ps := range psubs {
		subch, err := ps.Subscribe("foobar")
		if err != nil {
			t.Fatal(err)
		}

		msgs = append(msgs, subch)
	}
	sparseConnect(t, hosts)
	// wait for mesh building
	time.Sleep(time.Millisecond * 50)
	for i := 0; i < 100; i++ {
		msg := []byte(fmt.Sprintf("%d it's not a floooooood %d", i, i))

		owner := 0

		err := psubs[owner].Publish("foobar", msg)
		if err != nil {
			t.Fatal(err)
		}

		for _, sub := range msgs {
			got, err := sub.Next(ctx)
			if err != nil {
				t.Fatal(sub.err)
			}
			if !bytes.Equal(msg, got.Data) {
				t.Fatal("got wrong message!")
			}
		}
	}
}

func TestDensePlumsub(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hosts := getNetHosts(t, ctx, 10)

	psubs := getPlumsubs(ctx, hosts)

	var msgs []*Subscription
	for _, ps := range psubs {
		subch, err := ps.Subscribe("foobar")
		if err != nil {
			t.Fatal(err)
		}

		msgs = append(msgs, subch)
	}

	denseConnect(t, hosts)
	// wait for mesh building
	time.Sleep(time.Millisecond * 50)
	for i := 0; i < 10; i++ {
		msg := []byte(fmt.Sprintf("%d it's not a floooooood %d", i, i))

		// owner := rand.Intn(len(psubs))
		owner := 0

		psubs[owner].Publish("foobar", msg)

		for _, sub := range msgs {
			got, err := sub.Next(ctx)
			if err != nil {
				t.Fatal(sub.err)
			}
			if !bytes.Equal(msg, got.Data) {
				t.Fatal("got wrong message!")
			}
		}
	}
}

func TestPlumsubFanout(t *testing.T) {
	// No Fanout, subscribe a topic before use plumsub
}

func TestPlumsubFanoutMaintenance(t *testing.T) {
	// No FanoutMaintenance, subscribe a topic before use plumsub
}

func TestPlumsubPrune(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hosts := getNetHosts(t, ctx, 10)

	psubs := getPlumsubs(ctx, hosts)

	var msgs []*Subscription
	for _, ps := range psubs {
		subch, err := ps.Subscribe("foobar")
		if err != nil {
			t.Fatal(err)
		}

		msgs = append(msgs, subch)
	}

	denseConnect(t, hosts)
	// wait for mesh building
	time.Sleep(time.Millisecond * 50)
	const disconnect = 2
	// disconnect some peers from the mesh to get some PRUNEs
	for _, sub := range msgs[len(msgs)-disconnect:] {
		sub.Cancel()
	}

	// wait a bit to take effect
	time.Sleep(time.Millisecond * 100)

	for i := 0; i < 100; i++ {
		msg := []byte(fmt.Sprintf("%d it's not a floooooood %d", i, i))

		// owner := rand.Intn(len(psubs))
		owner := 1

		psubs[owner].Publish("foobar", msg)

		for _, sub := range msgs[:len(msgs)-disconnect] {
			got, err := sub.Next(ctx)
			if err != nil {
				t.Fatal(sub.err)
			}
			if !bytes.Equal(msg, got.Data) {
				t.Fatal("got wrong message!")
			}
		}
		// printRouters(psubs)
	}
}

func TestPlumsubGraft(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hosts := getNetHosts(t, ctx, 20)

	psubs := getPlumsubs(ctx, hosts)

	sparseConnect(t, hosts)
	// wait for mesh building
	time.Sleep(time.Millisecond * 50)
	var msgs []*Subscription
	for _, ps := range psubs {
		subch, err := ps.Subscribe("foobar")
		if err != nil {
			t.Fatal(err)
		}

		msgs = append(msgs, subch)

		// wait for announce to propagate
		time.Sleep(time.Millisecond * 100)
	}

	for i := 0; i < 100; i++ {
		msg := []byte(fmt.Sprintf("%d it's not a floooooood %d", i, i))

		owner := rand.Intn(len(psubs))

		psubs[owner].Publish("foobar", msg)

		for _, sub := range msgs {
			got, err := sub.Next(ctx)
			if err != nil {
				t.Fatal(sub.err)
			}
			if !bytes.Equal(msg, got.Data) {
				t.Fatal("got wrong message!")
			}
		}
	}
}

func TestPlumsubRemovePeer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hosts := getNetHosts(t, ctx, 20)

	psubs := getPlumsubs(ctx, hosts)

	var msgs []*Subscription
	for _, ps := range psubs {
		subch, err := ps.Subscribe("foobar")
		if err != nil {
			t.Fatal(err)
		}

		msgs = append(msgs, subch)
	}

	denseConnect(t, hosts)
	// wait for mesh building
	time.Sleep(time.Millisecond * 50)
	// disconnect some peers to exercise RemovePeer paths
	for _, host := range hosts[:5] {
		host.Close()
	}

	for i := 0; i < 10; i++ {
		msg := []byte(fmt.Sprintf("%d it's not a floooooood %d", i, i))

		owner := 5 + rand.Intn(len(psubs)-5)

		psubs[owner].Publish("foobar", msg)

		for _, sub := range msgs[5:] {
			got, err := sub.Next(ctx)
			if err != nil {
				t.Fatal(sub.err)
			}
			if !bytes.Equal(msg, got.Data) {
				t.Fatal("got wrong message!")
			}
		}
	}
}

func TestPlumsubGraftPruneRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hosts := getNetHosts(t, ctx, 10)
	psubs := getPlumsubs(ctx, hosts)
	denseConnect(t, hosts)
	// wait for mesh building
	time.Sleep(time.Millisecond * 50)
	var topics []string
	var msgs [][]*Subscription
	for i := 0; i < 35; i++ {
		topic := fmt.Sprintf("topic%d", i)
		topics = append(topics, topic)

		var subs []*Subscription
		for _, ps := range psubs {
			subch, err := ps.Subscribe(topic)
			if err != nil {
				t.Fatal(err)
			}

			subs = append(subs, subch)
		}
		msgs = append(msgs, subs)
	}

	for i, topic := range topics {
		msg := []byte(fmt.Sprintf("%d it's not a floooooood %d", i, i))

		owner := rand.Intn(len(psubs))

		psubs[owner].Publish(topic, msg)

		for _, sub := range msgs[i] {
			got, err := sub.Next(ctx)
			if err != nil {
				t.Fatal(sub.err)
			}
			if !bytes.Equal(msg, got.Data) {
				t.Fatal("got wrong message!")
			}
		}
	}
}

func TestPlumsubMultihops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hosts := getNetHosts(t, ctx, 6)

	psubs := getPlumsubs(ctx, hosts)

	connect(t, hosts[0], hosts[1])
	connect(t, hosts[1], hosts[2])
	connect(t, hosts[2], hosts[3])
	connect(t, hosts[3], hosts[4])
	connect(t, hosts[4], hosts[5])
	// wait for mesh building
	time.Sleep(time.Millisecond * 50)
	var subs []*Subscription
	// subscription before publishing
	for i := 0; i < 6; i++ {
		ch, err := psubs[i].Subscribe("foobar")
		if err != nil {
			t.Fatal(err)
		}
		subs = append(subs, ch)
	}
	msg := []byte("i like cats")
	err := psubs[0].Publish("foobar", msg)
	if err != nil {
		t.Fatal(err)
	}

	// last node in the chain should get the message
	select {
	case out := <-subs[4].ch:
		if !bytes.Equal(out.GetData(), msg) {
			t.Fatal("got wrong data")
		}
	case <-time.After(time.Second * 5):
		t.Fatal("timed out waiting for message")
	}
}

func TestPlumsubTreeTopology(t *testing.T) {
	conf := MakePlumsubDefaultConf()
	conf.missingTimeout = 300 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hosts := getNetHosts(t, ctx, 10)
	psubs := getPlumsubs(ctx, hosts)

	connect(t, hosts[0], hosts[1])
	connect(t, hosts[1], hosts[2])
	connect(t, hosts[1], hosts[4])
	connect(t, hosts[2], hosts[3])
	connect(t, hosts[0], hosts[5])
	connect(t, hosts[5], hosts[6])
	connect(t, hosts[5], hosts[8])
	connect(t, hosts[6], hosts[7])
	connect(t, hosts[8], hosts[9])
	// wait for mesh building
	time.Sleep(time.Millisecond * 50)
	/*
		[0] -> [1] -> [2] -> [3]
		 |      L->[4]
		 v
		[5] -> [6] -> [7]
		 |
		 v
		[8] -> [9]
	*/

	var chs []*Subscription
	for _, ps := range psubs {
		ch, err := ps.Subscribe("fizzbuzz")
		if err != nil {
			t.Fatal(err)
		}

		chs = append(chs, ch)
	}

	assertPeerLists(t, hosts, psubs[0], 1, 5)
	assertPeerLists(t, hosts, psubs[1], 0, 2, 4)
	assertPeerLists(t, hosts, psubs[2], 1, 3)

	checkMessageRouting(t, "fizzbuzz", []*PubSub{psubs[9], psubs[3]}, chs)
}

func TestBasicPlumsubGetResipientSpeakers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hosts := getNetHosts(t, ctx, 2)

	psubs := getPlumsubs(ctx, hosts)

	var msgs []*Subscription
	for _, ps := range psubs {
		subch, err := ps.Subscribe("foobar")
		if err != nil {
			t.Fatal(err)
		}
		msgs = append(msgs, subch)
	}
	sparseConnect(t, hosts)
	// wait for mesh building
	time.Sleep(time.Millisecond * 50)
	msg := []byte(fmt.Sprintf(" it's not a floooooood "))
	err := psubs[0].Publish("foobar", msg)
	assert.NoError(t, err)
	for i, sub := range msgs {
		got, err := sub.Next(ctx)
		if err != nil {
			t.Fatal(sub.err)
		}
		if !bytes.Equal(msg, got.Data) {
			t.Fatal("got wrong message!")
		}
		rt, ok := psubs[i].rt.(*RoutablePlumRouter)
		if !ok {
			assert.FailNow(t, "not reactive router")
		}
		pid, err := rt.GetForwardFrom(got.Message)
		assert.NoError(t, err)
		assert.Equal(t, psubs[0].host.ID(), pid)
		pids, err := rt.GetForwardTo(got.Message)
		assert.NoError(t, err)
		if i == 0 {
			assert.Equal(t, []peer.ID{psubs[1].host.ID()}, pids)
		}
		if i == 1 {
			assert.Equal(t, []peer.ID{}, pids)
		}
	}
}
