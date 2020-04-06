# go-libp2p-pubsub

[![](https://img.shields.io/badge/made%20by-Protocol%20Labs-blue.svg?style=flat-square)](http://protocol.ai)
[![](https://img.shields.io/badge/project-libp2p-yellow.svg?style=flat-square)](http://github.com/libp2p/libp2p)
[![](https://img.shields.io/badge/freenode-%23libp2p-yellow.svg?style=flat-square)](http://webchat.freenode.net/?channels=%23libp2p)
[![Discourse posts](https://img.shields.io/discourse/https/discuss.libp2p.io/posts.svg)](https://discuss.libp2p.io)

This repo contains the canonical pubsub implementation for libp2p. We currently provide three message router options:
- Floodsub, which is the baseline flooding protocol.
- Randomsub, which is a simple probabilistic router that propagates to random subsets of peers.
- Gossipsub, which is a more advanced router with mesh formation and gossip propagation. See [spec](https://github.com/libp2p/specs/tree/master/pubsub/gossipsub) and  [implementation](https://github.com/libp2p/go-libp2p-pubsub/blob/master/gossipsub.go) for more details.

**PSA: The Hardening Extensions for Gossipsub (Gossipsub V1.1) can be found under development at https://github.com/libp2p/go-libp2p-pubsub/pull/263**

## Repo Lead Maintainer

[@vyzo](https://github.com/vyzo/)

> This repo follows the [Repo Lead Maintainer Protocol](https://github.com/ipfs/team-mgmt/blob/master/LEAD_MAINTAINER_PROTOCOL.md)  

## Table of Contents

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Install](#install)
- [Usage](#usage)
- [Implementations](#implementations)
- [Documentation](#documentation)
- [Tracing](#tracing)
- [Contribute](#contribute)
- [License](#license)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Install

```
go get github.com/bdware/go-libp2p-pubsub
```

## Usage

To be used for messaging in p2p instrastructure (as part of libp2p) such as BDWare, IPFS, Ethereum, other blockchains, etc.

### Examples

`To be written`

## Documentation

See the [libp2p specs](https://github.com/libp2p/specs/tree/master/pubsub) for high level documentation
and [API documentation](https://pkg.go.dev/github.com/bdware/go-libp2p-pubsub).

### In this repo, you will find

```
.
├── LICENSE
├── README.md
# Regular Golang repo set up
├── codecov.yml
├── pb
├── go.mod
├── go.sum
├── doc.go
# PubSub base
├── pubsub.go
├── blacklist.go
├── notify.go
├── comm.go
├── discovery.go
├── sign.go
├── subscription.go
├── topic.go
├── trace.go
├── tracer.go
├── validation.go
# Floodsub router
├── floodsub.go
# Randomsub router 
├── randomsub.go
# Gossipsub router 
├── gossipsub.go
└── mcache.go
```

### Tracing

The pubsub system supports _tracing_, which collects all events pertaining to the internals of the system. This allows you to recreate the complete message flow and state of the system for analysis purposes.

To enable tracing, instantiate the pubsub system using the `WithEventTracer` option; the option accepts a tracer with three available implementations in-package (trace to json, pb, or a remote peer).
If you want to trace using a remote peer, you can do so using the `traced` daemon from [go-libp2p-pubsub-tracer](https://github.com/libp2p/go-libp2p-pubsub-tracer). The package also includes a utility program, `tracestat`, for analyzing the traces collected by the daemon.

For instance, to capture the trace as a json file, you can use the following option:
```go
pubsub.NewGossipSub(..., pubsub.NewEventTracer(pubsub.NewJSONTracer("/path/to/trace.json")))
```

To capture the trace as a protobuf, you can use the following option:
```go
pubsub.NewGossipSub(..., pubsub.NewEventTracer(pubsub.NewPBTracer("/path/to/trace.pb")))
```

Finally, to use the remote tracer, you can use the following incantations:
```go
// assuming that your tracer runs in x.x.x.x and has a peer ID of QmTracer
pi, err := peer.AddrInfoFromP2pAddr(ma.StringCast("/ip4/x.x.x.x/tcp/4001/p2p/QmTracer"))
if err != nil {
  panic(err)
}

tracer, err := pubsub.NewRemoteTracer(ctx, host, pi)
if err != nil {
  panic(err)
}

ps, err := pubsub.NewGossipSub(..., pubsub.WithEventTracer(tracer))
```

## Contribute

Contributions welcome. Please check out [the issues](https://github.com/BDWare/go-libp2p-pubsub/issues).

Small note: If editing the README, please conform to the [standard-readme](https://github.com/RichardLitt/standard-readme) specification.

## License

The go-libp2p-pubsub project is dual-licensed under Apache 2.0 and MIT terms:

- Apache License, Version 2.0, ([LICENSE-APACHE](./LICENSE-APACHE) or http://www.apache.org/licenses/LICENSE-2.0)
- MIT license ([LICENSE-MIT](./LICENSE-MIT) or http://opensource.org/licenses/MIT)

Copyright for portions of this fork are held by [Jeromy Johnson, 2016] as part of the original [go-libp2p-pubsub](https://github.com/libp2p/go-libp2p-pubsub) project.

All other copyright for this fork are held by [The BDWare Authors, 2020].

All rights reserved.
