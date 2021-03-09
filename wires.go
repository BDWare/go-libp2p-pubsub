package pubsub

import (
	"context"
	"fmt"
	"sync"

	pubsub_pb "github.com/bdware/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
)

type TopicMsgHandler func(topic string, from peer.ID, data []byte)

type TopicWireListener interface {
	HandlePeerUp(p peer.ID, topic string)
	HandlePeerDown(p peer.ID, topic string)
}

type TopicWireListenerAdapter struct {
	Up, Down func(peer.ID, string)
}

func (t *TopicWireListenerAdapter) HandlePeerUp(p peer.ID, topic string) {
	t.Up(p, topic)
}
func (t *TopicWireListenerAdapter) HandlePeerDown(p peer.ID, topic string) {
	t.Down(p, topic)
}

/*===========================================================================*/

type PubSubTopicWiresRouter struct {
	wires     *PubSubTopicWires
	protocols []protocol.ID
}

func (r *PubSubTopicWiresRouter) Protocols() []protocol.ID {
	return r.protocols
}
func (r *PubSubTopicWiresRouter) Attach(psub *PubSub) {
}
func (r *PubSubTopicWiresRouter) AddPeer(peer.ID, protocol.ID)                 {}
func (r *PubSubTopicWiresRouter) RemovePeer(peer.ID)                           {}
func (r *PubSubTopicWiresRouter) EnoughPeers(topic string, suggested int) bool { return true }
func (r *PubSubTopicWiresRouter) AcceptFrom(peer.ID) AcceptStatus              { return AcceptAll }
func (r *PubSubTopicWiresRouter) HandleRPC(rpc *RPC)                           {}
func (r *PubSubTopicWiresRouter) Publish(msg *Message)                         {}
func (r *PubSubTopicWiresRouter) Join(topic string)                            {}
func (r *PubSubTopicWiresRouter) Leave(topic string)                           {}

/*===========================================================================*/

type topicElement struct {
	*Topic
	*Subscription
	*TopicEventHandler
	cc func()
}

type PubSubTopicWires struct {
	rw       sync.RWMutex
	psub     *PubSub
	tmsghndl TopicMsgHandler
	listener TopicWireListener
	topics   map[string]*topicElement
}

func NewPubSubTopicWires(h host.Host, opts ...Option) (*PubSubTopicWires, error) {
	rt := &PubSubTopicWiresRouter{
		protocols: []protocol.ID{protocol.ID("/topic-wires/1.0.0")},
	}
	// Set the default message size.
	// As we using the pubsub system to build a topic overlay protocol, the default
	// message size is too small for response.
	// We have already considered the warning written in pubsub.go:450.
	// First, we have our own protocols to send and receive messages;
	// Second, the write-amplipfy phenomenon can be alleviated by sending lightweighted
	// requests and answering the only one primary node.
	// However, we still need a machinism to support download resume.
	opts = append([]Option{WithMaxMessageSize(10 * 1024 * 1024)}, opts...) // 10M
	psub, err := NewPubSub(context.Background(), h, rt,
		opts...,
	)
	if err != nil {
		return nil, err
	}
	wires := &PubSubTopicWires{
		rw:       sync.RWMutex{},
		psub:     psub,
		tmsghndl: func(string, peer.ID, []byte) {},
		listener: &TopicWireListenerAdapter{
			Up:   func(p peer.ID, s string) {},
			Down: func(p peer.ID, s string) {},
		},
		topics: make(map[string]*topicElement),
	}
	rt.wires = wires

	return wires, nil
}

func (w *PubSubTopicWires) ID() peer.ID {
	return w.psub.host.ID()
}
func (w *PubSubTopicWires) Join(topic string) error {
	Topic, err := w.psub.Join(topic)
	if err != nil {
		return err
	}
	thndl, err := Topic.EventHandler()
	if err != nil {
		return err
	}
	sub, err := Topic.Subscribe()
	if err != nil {
		return err
	}
	cctx, cc := context.WithCancel(context.Background())
	w.rw.Lock()
	defer w.rw.Unlock()
	w.topics[topic] = &topicElement{
		Topic:             Topic,
		Subscription:      sub,
		TopicEventHandler: thndl,
		cc:                cc,
	}
	go func(ctx context.Context, l TopicWireListener) {
		for {
			evt, err := thndl.NextPeerEvent(ctx)
			if err != nil {
				return
			}
			switch evt.Type {
			case PeerJoin:
				l.HandlePeerUp(evt.Peer, topic)
			case PeerLeave:
				l.HandlePeerDown(evt.Peer, topic)
			default:
				return
			}
		}
	}(cctx, w.listener)
	go func(ctx context.Context) {
		for {
			msg, err := sub.Next(ctx)
			if err != nil {
				return
			}
			w.handleMsg(msg)
		}
	}(cctx)
	return nil
}
func (w *PubSubTopicWires) Leave(topic string) error {
	w.rw.Lock()
	te, ok := w.topics[topic]
	if !ok {
		w.rw.Unlock()
		return fmt.Errorf("topic not joined")
	}
	delete(w.topics, topic)
	w.rw.Unlock()

	te.Subscription.Cancel()
	te.TopicEventHandler.Cancel()
	te.cc()
	err := te.Topic.Close()
	if err != nil {
		return err
	}
	return nil

}
func (w *PubSubTopicWires) Topics() []string {
	return w.psub.GetTopics()
}
func (w *PubSubTopicWires) Neighbors(topic string) []peer.ID {
	out := w.psub.ListPeers(topic)
	if out == nil {
		return []peer.ID{}
	}
	return out
}
func (w *PubSubTopicWires) SetListener(l TopicWireListener) {
	w.rw.Lock()
	defer w.rw.Unlock()
	w.listener = l
}
func (w *PubSubTopicWires) SendMsg(topic string, to peer.ID, data []byte) error {
	// fmt.Printf("%s send to %s with topic %s\n", w.ID().ShortString(), to.ShortString(), topic)
	errCh := make(chan error, 1)
	w.psub.eval <- func() {
		capTopic := topic
		rpcch, ok := w.psub.peers[to]
		if !ok {
			errCh <- fmt.Errorf("cannot find peer")
			close(errCh)
			return
		}
		msg := &pubsub_pb.Message{
			From:      []byte(w.psub.signID),
			Data:      data,
			Seqno:     w.psub.nextSeqno(),
			Topic:     &capTopic,
			Signature: nil,
			Key:       nil,
		}
		err := signMessage(w.psub.signID, w.psub.signKey, msg)
		if err != nil {
			errCh <- fmt.Errorf("sign msg error=%s", err)
			close(errCh)
			return
		}
		rpcch <- rpcWithMessages(msg)
		close(errCh) // send a nil error to SendMsg()
	}
	select {
	case err := <-errCh:
		return err
	}
}
func (w *PubSubTopicWires) SetTopicMsgHandler(th TopicMsgHandler) {
	w.rw.Lock()
	defer w.rw.Unlock()
	w.tmsghndl = th
}
func (w *PubSubTopicWires) Close() error {
	return nil
}

func (w *PubSubTopicWires) handleMsg(msg *Message) {
	w.rw.RLock()
	hndl := w.tmsghndl
	w.rw.RUnlock()
	go hndl(*msg.Topic, msg.ReceivedFrom, msg.Data)
}
