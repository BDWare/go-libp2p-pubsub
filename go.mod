module bdware.org/libp2p/go-libp2p-pubsub

go 1.14

replace github.com/libp2p/go-libp2p-pubsub => ./ // v0.2.6-bdw1

require (
	github.com/gogo/protobuf v1.3.1
	github.com/ipfs/go-log v0.0.1
	github.com/libp2p/go-libp2p-blankhost v0.1.4
	github.com/libp2p/go-libp2p-core v0.3.1
	github.com/libp2p/go-libp2p-discovery v0.2.0
	github.com/libp2p/go-libp2p-pubsub v0.0.0-00010101000000-000000000000
	github.com/libp2p/go-libp2p-swarm v0.2.2
	github.com/multiformats/go-multiaddr v0.2.1
	github.com/multiformats/go-multistream v0.1.1
	github.com/stretchr/testify v1.4.0
	github.com/whyrusleeping/timecache v0.0.0-20160911033111-cfcb2f1abfee
	golang.org/x/net v0.0.0-20190620200207-3b0461eec859
)
