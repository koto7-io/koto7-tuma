# Contributing

Tuma is MIT. The useful first change is a source adapter.

## Add an adapter

Adapters live in `internal/adapters`. A source type is a `SourceAdapter`:

```go
type SourceAdapter interface {
    Verify(headers http.Header, body []byte, secret string) bool
    ExtractEventID(headers http.Header, body []byte) string
}
```

`Verify` checks the provider's signature and returns false for anything else. `ExtractEventID` returns the provider's id for that delivery. Inbound dedup is `UNIQUE(connection_id, provider_event_id)`, so a missing or unstable id stores duplicates. If the provider has no id, hash the body the way `hashBody` does.

Register the adapter in `registry` under the `source_type` string the API already allows, or add that string to the connections check in a new migration. Do not edit a migration that has already shipped.

Stripe, GitHub, EasyPost, generic HMAC, and internal are the existing ones. EasyPost is the short example: `internal/adapters/easypost.go` and `easypost_test.go`.

## Tests

A signed fixture, or the header format written down next to the test. Cover a valid signature, a bad signature, and the event id you expect. `go test ./internal/adapters`.

## Pull request

- The change stays in `internal/adapters` plus a migration if the source type is new.
- Do not change `internal/workflow` to support one provider. Delivery is the same POST for every source.
- Do not commit secrets, `.env`, or anything under `documents/`.
