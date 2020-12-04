package pubsub

import (
	"context"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/assert"
)

func mustNewTopicWires(h host.Host) *PubSubTopicWires {
	w, err := NewPubSubTopicWires(h)
	if err != nil {
		panic(err)
	}
	return w
}

func mustJoin(w *PubSubTopicWires, t string) {
	err := w.Join(t)
	if err != nil {
		panic(err)
	}
}

func TestBasicJoinAndLeave(t *testing.T) {
	hosts := getNetHosts(t, context.Background(), 2)
	connectAll(t, hosts)
	h1, h2 := hosts[0], hosts[1]
	tbo1 := mustNewTopicWires(h1)
	tbo2 := mustNewTopicWires(h2)
	topic := "test"

	t.Run("join", func(t *testing.T) {
		var err error
		err = tbo1.Join(topic)
		assert.NoError(t, err)
		err = tbo2.Join(topic)
		assert.NoError(t, err)
		time.Sleep(50 * time.Millisecond)
		assert.Equal(t, []peer.ID{h2.ID()}, tbo1.Neighbors(topic))
		assert.Equal(t, []peer.ID{h1.ID()}, tbo2.Neighbors(topic))
	})
	t.Run("leave", func(t *testing.T) {
		var err error
		err = tbo1.Leave(topic)
		assert.NoError(t, err)
		err = tbo2.Leave(topic)
		assert.NoError(t, err)
		time.Sleep(50 * time.Millisecond)
		assert.Equal(t, []peer.ID{}, tbo1.Neighbors(topic))
		assert.Equal(t, []peer.ID{}, tbo2.Neighbors(topic))
	})
}

func TestSendRecv(t *testing.T) {
	hosts := getNetHosts(t, context.Background(), 2)
	connectAll(t, hosts)
	h1, h2 := hosts[0], hosts[1]
	tbo1 := mustNewTopicWires(h1)
	tbo2 := mustNewTopicWires(h2)
	topic := "test"
	data := []byte("hello")
	okCH := make(chan struct{}, 1)
	tbo2.SetTopicMsgHandler(func(top string, from peer.ID, dat []byte) {
		assert.Equal(t, topic, top)
		assert.Equal(t, h1.ID(), from)
		assert.Equal(t, data, dat)
		okCH <- struct{}{}
	})

	mustJoin(tbo1, topic)
	mustJoin(tbo2, topic)
	err := tbo1.SendMsg(topic, h2.ID(), data)
	assert.NoError(t, err)
	select {
	case <-okCH:
	case <-time.After(1 * time.Second):
		t.Fatal("cannot receive msg")
	}

}
