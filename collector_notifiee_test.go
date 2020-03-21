package pubsub

import (
	"context"
	"fmt"
	"github.com/libp2p/go-libp2p-core/peer"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/stretchr/testify/assert"
	"sync"
	"testing"
	"time"
)

type mockNotifiee struct {
	m      sync.Mutex
	n      int
	t      *testing.T
	expect []ntcase
}

type ntcase struct {
	name string
	p    peer.ID
	data []byte
}

func newMockNotifiee(t *testing.T, expect []ntcase) *mockNotifiee {
	return &mockNotifiee{
		m:      sync.Mutex{},
		n:      0,
		t:      t,
		expect: expect,
	}
}
func (m *mockNotifiee) check(name string, p peer.ID, msg *pb.Message) {
	m.m.Lock()
	defer m.m.Unlock()
	if m.n >= len(m.expect) {
		m.t.Fatal("not enough expect event")
		return
	}
	expect := m.expect[m.n]
	switch expect.name {
	case "":
		fmt.Println("name:", name, "peer:", p.ShortString(), "msg:", msg)
	default:
		m.exactCheck(name, p, msg)
	}
	m.n++
}
func (m *mockNotifiee) exactCheck(name string, p peer.ID, msg *pb.Message) {
	expect := m.expect[m.n]
	assert.Equal(m.t, expect.name, name)
	switch name {
	case "recvUnseen":
		fallthrough
	case "recvSeen":
		fallthrough
	case "haveSent":
		assert.Equal(m.t, expect.p, p)
		assert.Equal(m.t, expect.data, msg.GetData())
	case "sentAll":
		assert.Equal(m.t, expect.data, msg.GetData())
	case "peerDown":
		assert.Equal(m.t, expect.p, p)
	default:
		m.t.Fatal(fmt.Sprintf("unknown expect name: %s", expect.name))
	}
}
func (m *mockNotifiee) finalCheck() {
	if m.n != len(m.expect) {
		m.t.Fatal(fmt.Sprintf("some expected events didn't take place"))
	}
}

func (m *mockNotifiee) OnRecvUnseenMessage(from peer.ID, msg *pb.Message) {
	m.check("recvUnseen", from, msg)
}
func (m *mockNotifiee) OnRecvSeenMessage(from peer.ID, msg *pb.Message) {
	m.check("recvSeen", from, msg)
}
func (m *mockNotifiee) OnHaveSentMessage(to peer.ID, msg *pb.Message) {
	m.check("haveSent", to, msg)
}
func (m *mockNotifiee) OnHaveSentAll(msg *pb.Message) {
	m.check("sentAll", peer.ID(""), msg)
}
func (m *mockNotifiee) OnPeerDown(p peer.ID) {
	m.check("peerDown", p, nil)
}

func TestBasicCollect(t *testing.T) {
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
	msgdata := []byte("it's not a floooooood")
	serial0 := []ntcase{
		ntcase{
			name: "recvUnseen",
			p:    psubs[0].host.ID(),
			data: msgdata,
		},
		ntcase{
			name: "haveSent",
			p:    psubs[1].host.ID(),
			data: msgdata,
		},
		ntcase{
			name: "sentAll",
			p:    peer.ID(""),
			data: msgdata,
		},
	}
	serial1 := []ntcase{
		ntcase{
			name: "recvUnseen",
			p:    psubs[0].host.ID(),
			data: msgdata,
		},
		ntcase{
			name: "sentAll",
			p:    peer.ID(""),
			data: msgdata,
		},
	}
	mn0 := newMockNotifiee(t, serial0)
	psubs[0].SetCollectorNotifiee(mn0)
	mn1 := newMockNotifiee(t, serial1)
	psubs[1].SetCollectorNotifiee(mn1)
	err := psubs[0].Publish("foobar", msgdata)
	if err != nil {
		t.Fatal(err)
	}
	// wait for message propagation
	time.Sleep(50 * time.Millisecond)
	mn0.finalCheck()
	mn1.finalCheck()
}
