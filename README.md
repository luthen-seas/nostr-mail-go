# nostr-mail-go

Go implementation of the NOSTR Mail protocol — built independently from the spec for interoperability validation.

## Install

```bash
go get github.com/nostr-mail/nostr-mail-go
```

## Packages

| Package | Purpose |
|---------|---------|
| `pkg/mail` | Kind 1400 event creation and parsing |
| `pkg/wrap` | NIP-59 seal + gift wrap (send/receive) |
| `pkg/thread` | Thread tree reconstruction |
| `pkg/spam` | Anti-spam tier evaluation |
| `pkg/state` | Mailbox state (G-Set reads, LWW folders) |
| `cmd/interop` | Cross-implementation interop test CLI |

## Quick Start

```go
import "github.com/nostr-mail/nostr-mail-go/pkg/mail"

rumor := mail.CreateRumor(mail.CreateParams{
    SenderPubKey: alicePubkey,
    Recipients:   []mail.Recipient{{PubKey: bobPubkey, Role: "to"}},
    Subject:      "Hello",
    Body:         "Hello from Go!",
})
```

## Interop Testing

```bash
go run ./cmd/interop/
```

Runs conformance tests against shared test vectors and outputs a JSON report.

## License

MIT


---

## Project Layout — NOSTR Mail Ecosystem

The NOSTR Mail project is split across six repositories with clear ownership of each artifact:

| Repo | Source of truth for | This repo? |
|---|---|---|
| [nostr-mail-spec](https://github.com/luthen-seas/nostr-mail-spec) | Living spec, threat model, decisions log, design docs |  |
| [nostr-mail-nip](https://github.com/luthen-seas/nostr-mail-nip) | Submission-ready NIP draft, **canonical test vectors** |  |
| [nostr-mail-ts](https://github.com/luthen-seas/nostr-mail-ts) | TypeScript reference implementation |  |
| [nostr-mail-go](https://github.com/luthen-seas/nostr-mail-go) | Go second implementation (interop) | ✅ |
| [nostr-mail-bridge](https://github.com/luthen-seas/nostr-mail-bridge) | SMTP ↔ NOSTR gateway |  |
| [nostr-mail-client](https://github.com/luthen-seas/nostr-mail-client) | Reference web client (SvelteKit) |  |

**Test vectors** are canonical in `nostr-mail-nip/test-vectors/` and consumed by the implementation repos via git submodule. Do not edit a local copy in an impl repo — submit changes to `nostr-mail-nip`.

See [CONTRIBUTING.md](CONTRIBUTING.md) for the cross-repo contribution workflow, [SECURITY.md](SECURITY.md) for vulnerability reporting, and [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) for community standards.
