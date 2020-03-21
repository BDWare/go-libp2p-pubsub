package pubsub

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"io"
	"time"
)

const (
	// PlumSubProtocol is the protocol id of plumsub
	PlumSubProtocol = protocol.ID("/plumsub/1.0.0")
)

var (
	// ErrSenderNotFound .
	ErrSenderNotFound = errors.New("ErrSenderNotFound")
	// ErrRouterNotFound .
	ErrRouterNotFound = errors.New("ErrRouterNotFound")
	// ErrUnsupportTopicNum .
	ErrUnsupportTopicNum = errors.New("ErrUnsupportTopicNum")
	// ErrUnsupportRouter .
	ErrUnsupportRouter = errors.New("ErrUnsupportRouter")
)

var (
	plog = logging.Logger("plumsub")
)

// PlumConf is PlumRouter's configuration
type PlumConf struct {
	InitialDelay        time.Duration
	McacheShiftDuration time.Duration
	missingTimeout      time.Duration
	reactCacheDuration  time.Duration
	HopOptimization     bool
	HopThreshold        int
	McacheSize          int
	ResponseRouterSize  int
	EagerFanout         int
	LazyFanout          int
}

// MakePlumsubDefaultConf return the default conf of plumrouter
func MakePlumsubDefaultConf() PlumConf {
	return PlumConf{
		InitialDelay:        100 * time.Millisecond,
		McacheShiftDuration: 10 * time.Second,
		missingTimeout:      100 * time.Millisecond,
		reactCacheDuration:  10 * time.Second,
		ResponseRouterSize:  100,
		McacheSize:          10,
		EagerFanout:         4,
		LazyFanout:          5,
		HopOptimization:     true,
		HopThreshold:        4,
	}
}

//WithCustomPlumsubConf allows to customize plumsub's configuration
func WithCustomPlumsubConf(cfg PlumConf) Option {
	return func(psub *PubSub) error {
		rt, ok := psub.rt.(*PlumRouter)
		if !ok {
			return ErrUnsupportRouter
		}
		err := check(cfg)
		if err != nil {
			return fmt.Errorf("cfg check failed: err: %w cfg:%v", err, cfg)
		}
		rt.conf = cfg
		return nil
	}
}

// WithRoutable makes plumRouter responsive to GetForwardTo GetForwardFrom
func WithRoutable() Option {
	return func(psub *PubSub) error {
		rt, ok := psub.rt.(*PlumRouter)
		if !ok {
			return ErrUnsupportRouter
		}
		psub.rt = NewRoutablePlumRouter(rt)
		return nil
	}
}

func check(c PlumConf) error {
	// TODO: add config check
	return nil
}

// PlumRouter use plumtree as pubsub's router
// notice: subscribe before publishing, as joining the topic-defined overlay
// todo: no need to subscribe before publishing
type PlumRouter struct {
	conf      PlumConf
	p         *PubSub
	rts       map[string]*singleTopicRouter
	emitcache map[peer.ID]*RPC // emitcache is for piggybacking control messages
	mcache    *MessageCache
	tracer    *pubsubTracer
}

// NewPlumRouter returns an instance of plumRouter
func NewPlumRouter(conf PlumConf) (*PlumRouter, error) {
	if err := check(conf); err != nil {
		return nil, err
	}
	pr := &PlumRouter{
		conf:      conf,
		mcache:    NewMessageCache(conf.McacheSize, conf.McacheSize),
		rts:       make(map[string]*singleTopicRouter),
		emitcache: make(map[peer.ID]*RPC),
	}
	return pr, nil
}

// NewPlumSub returns a new pubsub using plumrouter
func NewPlumSub(ctx context.Context, h host.Host, opts ...Option) (*PubSub, error) {

	pr, err := NewPlumRouter(MakePlumsubDefaultConf())
	if err != nil {
		return nil, err
	}
	return NewPubSub(ctx, h, pr, opts...)
}

// Protocols .
func (pr *PlumRouter) Protocols() []protocol.ID {
	return []protocol.ID{PlumSubProtocol}
}

// Attach .
func (pr *PlumRouter) Attach(psub *PubSub) {
	pr.p = psub
	pr.tracer = psub.tracer
	go pr.mcacheShiftLoop()
}

// AddPeer . Notice that a new peer may not join any topic; you cannot find the new
// peer in p.topics.
func (pr *PlumRouter) AddPeer(pid peer.ID, prot protocol.ID) {
	pr.tracer.AddPeer(pid, prot)
	if prot != PlumSubProtocol {
		return
	}
}

// RemovePeer .
func (pr *PlumRouter) RemovePeer(pid peer.ID) {
	plog.Debugf("%s: remove peer: %s", pr.id(), pid.ShortString())
	pr.tracer.RemovePeer(pid)
	for _, rt := range pr.rts {
		rt.neighborDown(pid)
	}
	pr.p.collectorNotifiee.OnPeerDown(pid)
}

// EnoughPeers .
func (pr *PlumRouter) EnoughPeers(topic string, suggested int) bool {
	return true
}

// HandleRPC .
func (pr *PlumRouter) HandleRPC(rpc *RPC) {
	ctrl := rpc.GetControl()
	for i, msg := range rpc.Publish {
		for _, topic := range msg.GetTopicIDs() {
			rounds := ctrl.GetRounds()
			var round int
			if 0 <= i && i <= len(rounds) {
				round = int(rounds[i])
			}
			if srt, ok := pr.rts[topic]; ok {
				plog.Debugf("%s: recv msg from %s", pr.id(), rpc.from.ShortString())
				srt.receiveGossip(msg, round, rpc.from)
			}
		}
	}
	grafts := ctrl.GetGraft()
	prunes := ctrl.GetPrune()
	ihaves := ctrl.GetIhave()
	for _, graft := range grafts {
		topic := graft.GetTopicID()
		round := int(graft.GetRound())
		rt, ok := pr.rts[topic]
		if !ok {
			continue
		}
		for _, mid := range graft.GetMessageIDs() {
			rt.receiveGraft(mid, round, rpc.from)
		}
	}
	for _, prune := range prunes {
		topic := prune.GetTopicID()
		rt, ok := pr.rts[topic]
		if !ok {
			continue
		}
		rt.receivePrune(rpc.from)
	}
	for _, ihave := range ihaves {
		topic := ihave.GetTopicID()
		round := int(ihave.GetRound())
		rt, ok := pr.rts[topic]
		if !ok {
			continue
		}
		for _, mid := range ihave.GetMessageIDs() {
			rt.receiveIhave(mid, round, rpc.from)
		}
	}
}

// Publish .
func (pr *PlumRouter) Publish(from peer.ID, msg *pb.Message) {
	topics := msg.GetTopicIDs()
	for _, topic := range topics {
		rt, ok := pr.rts[topic]
		if !ok {
			plog.Debugf("%s: no router for topic:%s", pr.id(), topic)
			continue
		}
		if pr.p.host.ID() == from {
			plog.Debugf("%s: publish msg", pr.id())
			pr.p.collectorNotifiee.OnRecvUnseenMessage(from, msg)
			rt.broadcast(msg)
			pr.p.collectorNotifiee.OnHaveSentAll(msg)
		} else {
			// i have no idea where can i get the round, leave receiveGossip in
			// handleRPC
		}
	}
}

//Join .
func (pr *PlumRouter) Join(topic string) {
	plog.Debugf("%s: join %s", pr.id(), topic)
	_, ok := pr.rts[topic]
	if ok {
		panic(fmt.Sprintf("unexpected rt with topic:%s", topic))
	}
	pr.tracer.Join(topic)
	pr.rts[topic] = newSingleTopicRouter(topic, pr)
}

// Leave .
func (pr *PlumRouter) Leave(topic string) {
	plog.Debugf("%s: leave %s", pr.id(), topic)
	rt, ok := pr.rts[topic]
	if !ok {
		panic(fmt.Sprintf("unexpected nil rt with topic:%s", topic))
	}
	pr.tracer.Leave(topic)
	rt.leave()
	delete(pr.rts, topic)
}

// OnPeerJoin .
func (pr *PlumRouter) OnPeerJoin(topic string, pid peer.ID) {
	plog.Debugf("%s: know %s joins %s", pr.id(), pid.ShortString(), topic)
	srt, ok := pr.rts[topic]
	if ok {
		srt.neighborUp(pid)
	}
}

// OnPeerLeave .
func (pr *PlumRouter) OnPeerLeave(topic string, pid peer.ID) {
	plog.Debugf("%s: know %s leaves %s", pr.id(), pid.ShortString(), topic)
	srt, ok := pr.rts[topic]
	if ok {
		srt.neighborDown(pid)
	}
}

func (pr *PlumRouter) mcacheShiftLoop() {
	time.Sleep(pr.conf.InitialDelay)
	ticker := time.NewTicker(pr.conf.McacheShiftDuration)
	defer ticker.Stop()
	pr.eval(pr.mcacheShift)
	for {
		select {
		case <-pr.p.ctx.Done():
			return
		case <-ticker.C:
			pr.eval(pr.mcacheShift)
		}
	}
}

func (pr *PlumRouter) mcacheShift() {
	pr.mcache.Shift()
}

func (pr *PlumRouter) tagPeer(p peer.ID, topic string) {
	tag := fmt.Sprintf("plumsub:%s", topic)
	pr.p.host.ConnManager().TagPeer(p, tag, 2)
}

func (pr *PlumRouter) untagPeer(p peer.ID, topic string) {
	tag := fmt.Sprintf("plumsub:%s", topic)
	pr.p.host.ConnManager().UntagPeer(p, tag)
}

func (pr *PlumRouter) id() string {
	return pr.self().ShortString()
}

func (pr *PlumRouter) self() peer.ID {
	return pr.p.host.ID()
}

func (pr *PlumRouter) eval(thunk func()) bool {
	select {
	case pr.p.eval <- thunk:
		return true
	case <-pr.p.ctx.Done():
		return false
	}
}
func (pr *PlumRouter) syncEval(thunk func()) bool {
	done := make(chan struct{})
	thunkWithChan := func() {
		thunk()
		select {
		case done <- struct{}{}:
			close(done)
		default:
		}
	}
	ok := pr.eval(thunkWithChan)
	if ok {
		<-done
	}
	return ok
}

func (pr *PlumRouter) loadOrAddRPC(pid peer.ID) *RPC {
	rpc, ok := pr.emitcache[pid]
	if !ok || rpc == nil {
		pr.emitcache[pid] = &RPC{
			RPC: pb.RPC{
				Publish: []*pb.Message{},
				Control: &pb.ControlMessage{
					Graft: []*pb.ControlGraft{},
					Prune: []*pb.ControlPrune{},
					Ihave: []*pb.ControlIHave{},
				},
			},
		}
	}
	return pr.emitcache[pid]
}

func (pr *PlumRouter) emitAll() {
	for pid := range pr.emitcache {
		pr.emitRPC(pid)
	}
}

func (pr *PlumRouter) emitRPC(pid peer.ID) {
	rpc, ok := pr.emitcache[pid]
	if !ok {
		panic(fmt.Sprintf("unexpected emit: cannot find rpc for pid:%s", pid.ShortString()))
	}
	if rpc == nil {
		panic(fmt.Sprintf("unexpected emit: cannot emit nil rpc with pid:%s", pid.ShortString()))
	}
	mch, ok := pr.p.peers[pid]
	if !ok {
		panic(fmt.Sprintf("unexpected emit: cannot find peer chan for pid:%s", pid.ShortString()))
	}
	select {
	case mch <- rpc:
		plog.Debugf("%s: send rpc to %s", pr.id(), pid.ShortString())
		pr.tracer.SendRPC(rpc, pid)
	default:
		plog.Infof("dropping message to peer %s: queue full", pid.ShortString())
		pr.tracer.DropRPC(rpc, pid)
	}
	delete(pr.emitcache, pid)
}

func piggyback(rpc *RPC, gossips []*pb.Message, rounds []uint32, grafts []*pb.ControlGraft, prunes []*pb.ControlPrune, ihaves []*pb.ControlIHave) {
	for i := range gossips {
		rpc.RPC.Publish = append(rpc.RPC.Publish, gossips[i])
	}
	for i := range rounds {
		rpc.RPC.Control.Rounds = append(rpc.RPC.Control.Rounds, rounds[i])
	}
	for i := range grafts {
		rpc.RPC.Control.Graft = append(rpc.RPC.Control.Graft, grafts[i])
	}
	for i := range prunes {
		rpc.RPC.Control.Prune = append(rpc.RPC.Control.Prune, prunes[i])
	}
	for i := range ihaves {
		rpc.RPC.Control.Ihave = append(rpc.RPC.Control.Ihave, ihaves[i])
	}
}

type peerSet map[peer.ID]struct{}

func (ps peerSet) Find(pid peer.ID) bool {
	_, ok := ps[pid]
	return ok
}
func (ps peerSet) Add(pid peer.ID) {
	ps[pid] = struct{}{}
}
func (ps peerSet) Delete(pid peer.ID) {
	delete(ps, pid)
}
func (ps peerSet) AddAll(others peerSet) {
	for p := range others {
		ps.Add(p)
	}
}

type singleTopicRouter struct {
	psub    *PubSub
	pr      *PlumRouter
	topic   string
	eager   peerSet
	lazy    peerSet
	missing missingMsgSet
	cancels cancelSet
}

type cancelSet map[string]chan struct{}

type missingMsgSet map[string]map[peer.ID]int
type missingMsg struct {
	mid   string
	pid   peer.ID
	round int
}

func (m missingMsgSet) add(missing missingMsg) {
	pid2round, ok := m[missing.mid]
	if !ok {
		pid2round = make(map[peer.ID]int)
		m[missing.mid] = pid2round
	} else {
		// we may receive same mid but different pid
	}
	_, ok = pid2round[missing.pid]
	if !ok {
		pid2round[missing.pid] = missing.round
	} else {
		panic(fmt.Sprintf("unexpected round with %v", missing))
	}
}
func (m missingMsgSet) pickOneThenDeleteByMsgID(mid string) (*missingMsg, bool) {
	pid2round, ok := m[mid]
	if !ok || len(pid2round) == 0 {
		return nil, false
	}
	var out *missingMsg
	for pid, round := range pid2round {
		out = &missingMsg{
			mid:   mid,
			pid:   pid,
			round: round,
		}
		break
	}
	delete(m[mid], out.pid)
	return out, true
}
func (m missingMsgSet) findByMsgID(mid string) []*missingMsg {
	var out []*missingMsg
	pid2round, ok := m[mid]
	if !ok || len(pid2round) == 0 {
		return out
	}
	for pid, round := range pid2round {
		out = append(out, &missingMsg{
			mid:   mid,
			pid:   pid,
			round: round,
		})
	}
	return out
}
func (m missingMsgSet) deleteByMsgID(mid string) {
	delete(m, mid)
}
func (m missingMsgSet) deleteByPeerID(pid peer.ID) {
	for _, pid2round := range m {
		delete(pid2round, pid)
	}
}

// functions below are implementation of origin plumtree
func newSingleTopicRouter(topic string, pr *PlumRouter) *singleTopicRouter {
	topicPeers, ok := pr.p.topics[topic]
	if !ok {
		topicPeers = peerSet{}
	}
	eager := peerSet{}
	eager.AddAll(topicPeers)
	return &singleTopicRouter{
		psub:    pr.p,
		pr:      pr,
		topic:   topic,
		eager:   eager,
		lazy:    peerSet{},
		missing: make(map[string]map[peer.ID]int),
		cancels: make(map[string]chan struct{}),
	}
}
func (sr *singleTopicRouter) leave() {
	for _, ch := range sr.cancels {
		ch <- struct{}{}
	}
}

// events (called from plumRouter)

func (sr *singleTopicRouter) broadcast(msg *pb.Message) {
	sr.eagerPush(msg, 0, sr.self())
	sr.lazyPush(msg, 0, sr.self())
	sr.markSeen(msg)
	sr.deliver(msg)
}
func (sr *singleTopicRouter) receiveSeenGossip(msg *pb.Message, round int, sender peer.ID) {
	plog.Debugf("%s: recv seen msg from %s", sr.pr.id(), sender.ShortString())
	sr.eager.Delete(sender)
	sr.lazy.Add(sender)
	sr.sendPrune(sender)
}
func (sr *singleTopicRouter) receiveUnseenGossip(msg *pb.Message, round int, sender peer.ID) {
	plog.Debugf("%s: recv unseen msg from %s", sr.pr.id(), sender.ShortString())
	mid := msgID(msg)
	sr.markSeen(msg)
	if len(sr.missing.findByMsgID(mid)) > 0 {
		sr.cancelTimer(mid)
	}
	sr.missing.deleteByMsgID(mid)
	sr.eagerPush(msg, round+1, sender)
	sr.lazyPush(msg, round+1, sender)
	sr.eager.Add(sender)
	sr.lazy.Delete(sender)
	if sr.pr.conf.HopOptimization {
		sr.optimize(mid, round, sender)
	}
	sr.deliver(msg)
}
func (sr *singleTopicRouter) receiveGossip(msg *pb.Message, round int, sender peer.ID) {
	if sr.seenMsg(msgID(msg)) {
		sr.psub.collectorNotifiee.OnRecvSeenMessage(sender, msg)
		sr.receiveSeenGossip(msg, round, sender)
		return
	}
	sr.psub.collectorNotifiee.OnRecvUnseenMessage(sender, msg)
	sr.receiveUnseenGossip(msg, round, sender)
	sr.psub.collectorNotifiee.OnHaveSentAll(msg)
}
func (sr *singleTopicRouter) receivePrune(sender peer.ID) {
	plog.Debugf("%s: recv prune from %s", sr.pr.id(), sender.ShortString())
	sr.pr.tracer.Prune(sender, sr.topic)
	sr.eager.Delete(sender)
	sr.lazy.Add(sender)
}
func (sr *singleTopicRouter) receiveGraft(mid string, round int, sender peer.ID) {
	plog.Debugf("%s: recv graft from %s", sr.pr.id(), sender.ShortString())
	sr.pr.tracer.Graft(sender, sr.topic)
	sr.eager.Add(sender)
	sr.lazy.Delete(sender)
	if sr.seenMsg(mid) {
		msg, ok := sr.getSeenMsg(mid)
		if !ok {
			panic("unexpected nil seen msg")
		}
		sr.sendGossip(msg, round, sender)
		sr.pr.emitAll()
	}
}
func (sr *singleTopicRouter) receiveIhave(mid string, round int, sender peer.ID) {
	plog.Debugf("%s: recv ihave from %s", sr.pr.id(), sender.ShortString())
	if !sr.seenMsg(mid) {
		if !sr.findTimer(mid) {
			sr.setupTimer(mid, sender)
		}
		sr.missing.add(missingMsg{
			mid:   mid,
			pid:   sender,
			round: round,
		})
	} else {
		msg, ok := sr.pr.mcache.Get(mid)
		if !ok {
			panic("unexpected missing msg")
		}
		sr.psub.collectorNotifiee.OnRecvSeenMessage(sender, msg)
	}
}
func (sr *singleTopicRouter) timer(mid string) {
	missing, ok := sr.missing.pickOneThenDeleteByMsgID(mid)
	if !ok {
		// lost connect to all previous lazy peers
		return
		// panic("unexpected find missing msg failed")
	}
	plog.Debugf("%s: timer: missing: [pid:%s]", sr.pr.id(), missing.pid.ShortString())
	sr.setupTimer(mid, missing.pid)
	sr.eager.Add(missing.pid)
	sr.lazy.Delete(missing.pid)
	sr.sendGraft(missing.mid, missing.round, missing.pid)
}
func (sr *singleTopicRouter) neighborDown(node peer.ID) {
	sr.eager.Delete(node)
	sr.lazy.Delete(node)
	sr.missing.deleteByPeerID(node)
	sr.pr.untagPeer(node, sr.topic)
}
func (sr *singleTopicRouter) neighborUp(node peer.ID) {
	sr.eager.Add(node)
	sr.pr.tagPeer(node, sr.topic)
}

// intertal procedure
func (sr *singleTopicRouter) dispatch() {
	// for we piggybacks iHave in sendIhave, do nothing here
}
func (sr *singleTopicRouter) eagerPush(msg *pb.Message, round int, sender peer.ID) {
	for p := range sr.eager {
		if p == sender {
			continue
		}
		sr.sendGossip(msg, round, p)
	}
}
func (sr *singleTopicRouter) lazyPush(msg *pb.Message, round int, sender peer.ID) {
	for p := range sr.lazy {
		if p == sender {
			continue
		}
		sr.enqueueLazy(msg, round, p)
	}
	sr.dispatch()
}

func (sr *singleTopicRouter) optimize(mid string, round int, sender peer.ID) {

	missings := sr.missing.findByMsgID(mid)
	for _, missing := range missings {
		if missing.round < round && round-missing.round > sr.pr.conf.HopThreshold {
			plog.Debugf("%s: optimize: miss round: %d, round: %d", sr.pr.id(), missing.round, round)
			plog.Debugf("%s: optimize: change %s to %s", sr.pr.id(), sender.ShortString(), missing.pid.ShortString())
			sr.sendGraft("", missing.round, missing.pid)
			sr.sendPrune(sender)
		}
	}
}
func (sr *singleTopicRouter) deliver(msg *pb.Message) {
	sr.pr.emitAll()
	// i don't know what deliver() should do after i read the paper
	// just emitAll !
}
func (sr *singleTopicRouter) sendGossip(msg *pb.Message, round int, to peer.ID) {
	sr.psub.collectorNotifiee.OnHaveSentMessage(to, msg)
	rpc := sr.pr.loadOrAddRPC(to)
	rounds := []uint32{uint32(round)}
	gossips := []*pb.Message{msg}
	piggyback(rpc, gossips, rounds, nil, nil, nil)
}
func (sr *singleTopicRouter) sendIhave(mid string, round int, to peer.ID) {
	rpc := sr.pr.loadOrAddRPC(to)
	round32 := uint32(round)
	ihaves := []*pb.ControlIHave{&pb.ControlIHave{
		TopicID:    &sr.topic,
		MessageIDs: []string{mid},
		Round:      &round32,
	}}
	piggyback(rpc, nil, nil, nil, nil, ihaves)
}
func (sr *singleTopicRouter) sendGraft(mid string, round int, to peer.ID) {
	rpc := sr.pr.loadOrAddRPC(to)
	round32 := uint32(round)
	grafts := []*pb.ControlGraft{&pb.ControlGraft{
		TopicID:    &sr.topic,
		MessageIDs: []string{mid},
		Round:      &round32,
	}}
	piggyback(rpc, nil, nil, grafts, nil, nil)
	sr.pr.emitAll()
}
func (sr *singleTopicRouter) sendPrune(to peer.ID) {
	rpc := sr.pr.loadOrAddRPC(to)
	prunes := []*pb.ControlPrune{&pb.ControlPrune{
		TopicID: &sr.topic,
	}}
	piggyback(rpc, nil, nil, nil, prunes, nil)
	sr.pr.emitAll()
}
func (sr *singleTopicRouter) findTimer(mid string) bool {
	_, ok := sr.cancels[mid]
	return ok
}
func (sr *singleTopicRouter) setupTimer(mid string, sender peer.ID) {
	cancel, ok := sr.cancels[mid]
	if !ok {
		cancel = make(chan struct{}, 1)
		sr.cancels[mid] = cancel
	}
	go func() {
		select {
		case <-cancel:
		case <-time.After(sr.pr.conf.missingTimeout):
			sr.pr.eval(func() { sr.timer(mid) })
		}
	}()
}
func (sr *singleTopicRouter) cancelTimer(mid string) {
	cc, ok := sr.cancels[mid]
	if !ok {
		panic(fmt.Sprintf("unexpected nil cancel with mid: %v", mid))
	}
	cc <- struct{}{}
	close(cc)
	delete(sr.cancels, mid)
}

func (sr *singleTopicRouter) seenMsg(mid string) bool {
	_, ok := sr.pr.mcache.Get(mid)
	return ok
}
func (sr *singleTopicRouter) getSeenMsg(mid string) (*pb.Message, bool) {
	return sr.pr.mcache.Get(mid)
}
func (sr *singleTopicRouter) markSeen(msg *pb.Message) {
	sr.pr.mcache.Put(msg)
}

func (sr *singleTopicRouter) self() peer.ID {
	return sr.psub.host.ID()
}
func (sr *singleTopicRouter) enqueueLazy(msg *pb.Message, round int, to peer.ID) {
	sr.psub.collectorNotifiee.OnHaveSentMessage(to, msg)
	sr.sendIhave(msgID(msg), round, to)
}

func (sr *singleTopicRouter) fprint(wr io.Writer) {
	w := bufio.NewWriter(wr)
	sr.pr.syncEval(func() {
		fmt.Fprintf(w, "topic: %s\n", sr.self().ShortString())
		fmt.Fprintf(w, "id: %s\n", sr.topic)
		fmt.Fprintf(w, "eager:\n")
		for peer := range sr.eager {
			fmt.Fprintf(w, "  %s,\n", peer.ShortString())
		}
		fmt.Fprintf(w, "lazy:\n")
		for peer := range sr.lazy {
			fmt.Fprintf(w, "  %s,\n", peer.ShortString())
		}
		fmt.Fprintf(w, "missing:\n")
		for mid, pid2round := range sr.missing {
			for pid, round := range pid2round {
				fmt.Fprintf(w, "  [pid:%s,r:%d,mid:%v]\n", pid.ShortString(), round, []byte(mid))
			}
		}
		fmt.Fprintf(w, "emitcache:\n")
		for pid, rpc := range sr.pr.emitcache {
			fmt.Fprintf(w, "[pid:%s,rpc:%v]", pid, rpc)
		}
		fmt.Fprintf(w, "\n\n")
		w.Flush()
	})
}

func (sr *singleTopicRouter) print(w io.Writer) {
	sr.fprint(w)
}

// RoutablePlumRouter add two methods for PlumRouter, which are used to gather responses
type RoutablePlumRouter struct {
	*PlumRouter
	senders map[string]peer.ID
}

// NewRoutablePlumRouter .
func NewRoutablePlumRouter(rt *PlumRouter) *RoutablePlumRouter {
	return &RoutablePlumRouter{
		PlumRouter: rt,
		senders:    make(map[string]peer.ID),
	}

}

// NewRoutablePlumSub .
func NewRoutablePlumSub(ctx context.Context, h host.Host, opts ...Option) (*PubSub, error) {
	prt, err := NewPlumRouter(MakePlumsubDefaultConf())
	if err != nil {
		return nil, err
	}
	rt := NewRoutablePlumRouter(prt)
	return NewPubSub(ctx, h, rt, opts...)
}

// Publish is reactivePlumRouter's override publish function, which records (mid, sender) pairs and set
// timer to delete it later
func (rpr *RoutablePlumRouter) Publish(from peer.ID, msg *pb.Message) {
	mid := msgID(msg)
	_, ok := rpr.senders[mid]
	if ok {
		panic("unexpected seen message")
	}
	rpr.senders[mid] = from
	// set up a time to delete sender later
	go func() {
		time.Sleep(rpr.PlumRouter.conf.reactCacheDuration)
		rpr.eval(func() {
			delete(rpr.senders, mid)
		})
	}()
	rpr.PlumRouter.Publish(from, msg)
}

func checkMessageTopicsNum(msg *pb.Message) error {
	topics := msg.GetTopicIDs()
	if len(topics) == 0 || len(topics) >= 2 {
		return ErrUnsupportTopicNum
	}
	return nil
}

// GetForwardFrom finds out which peer sends the msg to us directly
func (rpr *RoutablePlumRouter) GetForwardFrom(msg *pb.Message) (pid peer.ID, err error) {
	pidch := make(chan peer.ID, 1)
	errch := make(chan error, 1)
	rpr.eval(func() {
		sender, ok := rpr.senders[msgID(msg)]
		if !ok {
			errch <- ErrSenderNotFound
			close(errch)
		} else {
			pidch <- sender
			close(pidch)
		}
	})
	select {
	case err = <-errch:
	case pid = <-pidch:
	}
	return
}

// GetForwardTo returns which peers we want to send. This is also the listener set when collect results
func (rpr *RoutablePlumRouter) GetForwardTo(msg *pb.Message) (pids []peer.ID, err error) {
	err = checkMessageTopicsNum(msg)
	if err != nil {
		return
	}
	topics := msg.GetTopicIDs()
	topic := topics[0]
	errch := make(chan error, 1)
	pidsch := make(chan []peer.ID, 1)
	rpr.eval(func() {
		sender, ok := rpr.senders[msgID(msg)]
		if !ok {
			errch <- ErrSenderNotFound
			close(errch)
			return
		}
		rt, ok := rpr.rts[topic]
		if !ok {
			errch <- ErrRouterNotFound
			close(errch)
			return
		}
		eagers := rt.eager
		out := make([]peer.ID, 0, len(eagers))
		for pid := range eagers {
			if pid == sender {
				continue
			}
			out = append(out, pid)
		}
		pidsch <- out
		close(pidsch)
	})
	select {
	case err = <-errch:
	case pids = <-pidsch:
	}
	return
}

// Fprint prints routetable to wr
func (rpr *RoutablePlumRouter) Fprint(wr io.Writer) {
	for _, srt := range rpr.rts {
		srt.print(wr)
	}
}

// ActiveRoutablePlumRouter use RouteNotifiee to update listener set
type ActiveRoutablePlumRouter struct {
	*RoutablePlumRouter
	notifiees map[RouteNotifiee]struct{}
}

// RouteNotifiee interface.
type RouteNotifiee interface {
	// SendBegin indicates a new message come
	SendBegin(msg *pb.Message)
	// InitListeners indicates the init listener set
	InitListeners(msg *pb.Message, pids []peer.ID)
	// AddListeners indicates a new listener add to the set
	AddListeners(msg *pb.Message, pid peer.ID)
	// RemoveListeners indicates a listener may drop from the given set
	RemoveListeners(msg *pb.Message, pid peer.ID)
	// SendDone indicates the given set will not change
	SendDone(msg *pb.Message)
}

// Register .
func (arpr *ActiveRoutablePlumRouter) Register(notif RouteNotifiee) {
	arpr.eval(func() {
		arpr.notifiees[notif] = struct{}{}
	})
}

// Unregister .
func (arpr *ActiveRoutablePlumRouter) Unregister(notif RouteNotifiee) {
	arpr.eval(func() {
		delete(arpr.notifiees, notif)
	})
}
