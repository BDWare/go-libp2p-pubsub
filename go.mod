module github.com/bdware/go-libp2p-pubsub

go 1.14

replace github.com/libp2p/go-libp2p-pubsub => ./ // v0.3.1-bdw.1

require (
	github.com/benbjohnson/clock v1.0.1
	github.com/gogo/protobuf v1.3.1
	github.com/ipfs/go-log v1.0.4
	github.com/libp2p/go-libp2p-blankhost v0.1.6
	github.com/libp2p/go-libp2p-connmgr v0.2.3
	github.com/libp2p/go-libp2p-core v0.5.6
	github.com/libp2p/go-libp2p-discovery v0.4.0
	github.com/libp2p/go-libp2p-pubsub v0.3.1
	github.com/libp2p/go-libp2p-swarm v0.2.4
	github.com/multiformats/go-multiaddr v0.2.2
	github.com/multiformats/go-multiaddr-net v0.1.5
	github.com/multiformats/go-multistream v0.1.1
	github.com/stretchr/testify v1.4.0
	github.com/whyrusleeping/timecache v0.0.0-20160911033111-cfcb2f1abfee
)
