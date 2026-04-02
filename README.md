# nostr-mail-go

Go implementation of the NOSTR Mail protocol — built independently from the spec for interoperability validation.

## Install

```bash
go get github.com/nostr-mail/nostr-mail-go
```

## Packages

| Package | Purpose |
|---------|---------|
| `pkg/mail` | Kind 15 event creation and parsing |
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
