# fragcap

Facade crate re-exporting the fragcap library surface.

## Status

Functional as of 0.2.0, the first functional release. This crate re-exports the full fragcap library surface, from interface selection and the capture pipeline to the output writers, the profile schema, attribution, and Steam integration. The library is the product; install the `fragcap` binary from the `fragcap-cli` crate.

## About fragcap

fragcap is a passive, process-attributed network capture tool for Windows,
written in Rust. Packet capture is a solved problem; attribution is not.
Standard tooling captures at the network driver, below the socket layer, where
the association between a packet and the process that produced it has already
been discarded. fragcap reconstructs that association for game clients
launched indirectly through platform and publisher launchers, and writes it
into an extended pcapng profile that unmodified analyzers still read as
ordinary pcapng.

It observes. It does not modify traffic, and it does not reach inside the
processes it names.

## This crate

The library is the product; the command line tool is one consumer of it. Anything reachable through the command line is reachable through this crate. Install the `fragcap` binary from the `fragcap-cli` crate instead.

## Stable Deep Capture API

Enable the `deep-capture` feature and import new integrations from `fragcap::deep_capture::api`. That module carries the explicit version-one compatibility contract, checked builders, coordinator and adapter ownership, cooperative cancellation, production native backend entry point, observations, artifacts, recovery, and terminal reporting. Existing top-level `fragcap::deep_capture` exports remain available for compatibility but are not an additional stability promise.

`SessionConfigBuilder` and `AdapterSetBuilder` are the additive construction paths. `DeepCaptureSession`, adapter sets, and their injected trait objects are serial and thread-confined; `CancellationToken` is cloneable, `Send`, and `Sync`. Cancellation is checked before effects and between adapter calls. An in-flight adapter cannot be preempted and must honor its finite `Budget`.

Run the production native loopback example without the CLI or host trust changes:

```text
cargo run -p fragcap --example native-deep-capture --features deep-capture
```

## License

Apache-2.0. See [`LICENSE`](LICENSE) and [`NOTICE`](NOTICE).

npcap is required at runtime but is never bundled, downloaded, or installed by
fragcap, and its Software Development Kit is never vendored. Obtain it from
[npcap.com](https://npcap.com) and review its license terms there.

[repo]: https://github.com/h8rt3rmin8r/fragcap
