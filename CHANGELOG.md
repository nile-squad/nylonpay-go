# Changelog

All notable changes to `nylonpay-go` are documented here.

## [0.1.1] — 2026-09-06

### Changed

- Integration tests load a `.env` from the project root, falling back to
  `tests/integration/.env` when that is the one present, and log which file was
  loaded.
- README documents the credential setup and the `-tags=integration` flag the
  Windows invocations were missing — without it the suite is excluded at compile
  time and reports a passing run that tested nothing.
- `.env.example` gains optional `NYLONPAY_REVOKED_API_KEY` /
  `NYLONPAY_REVOKED_API_SECRET`, which enable I15 in live mode.

## [0.1.0] — 2026-09-05

First published release, tagged as `v0.1.0`. The module is pre-1.0, so the API
may still break between minor versions; the SDK spec it targets is versioned
separately, and this release brings the Go SDK up to Nylon Pay SDK spec v2.1.0.

It is a breaking overhaul of the untagged code that preceded it: that API could
not sign a request the backend would accept for a wide class of ordinary
payloads, and exposed no way for a merchant to branch on an error.

### Fixed — signing (any earlier version could not talk to the backend reliably)

- **Canonical payload is now RFC 8785 (JCS).** The previous implementation
  delegated to `encoding/json`, which HTML-escapes `<`, `>` and `&`. Any payload
  containing one of those — a description of `"Tea & Coffee"` was enough —
  produced a canonical string the server could not reproduce, failing with an
  opaque `auth` error every time. Conformance vector V4 covers this.
- **Object keys sort by UTF-16 code unit**, not by Go's native string order
  (which is UTF-8 byte order, i.e. code-point order). The two agree across the
  BMP and diverge above U+FFFF, so a merchant metadata key containing an emoji
  signed incorrectly.
- **Retries are signed fresh per attempt.** The nonce, timestamp and signature
  were computed once outside the retry loop, so every retry replayed a nonce the
  backend had already consumed and was rejected. The retry path was inert. The
  body — and therefore the reference — still stays constant across attempts, so
  idempotency is unaffected.

### Added — security

- **Response replay protection.** The backend echoes the request nonce inside
  the signed response as `_requestNonce`; the SDK now requires it to match the
  nonce it sent, and rejects the response as `internal` otherwise. Without this
  a captured response stayed validly signed forever and could be replayed onto a
  later call for the same reference.
- **Response size cap**, default 10 MB, enforced while the body is read against
  a running byte count. A `Content-Length` check alone cannot bound peak memory
  and is a no-op for chunked responses.
- **Canonical signature form.** Signatures are accepted as lowercase hex only.
  Comparing decoded bytes is case-blind and would accept the same digest spelled
  a second way.
- **Conformance vectors V1–V7** as a unit test (spec requirement S19), plus a
  regression test for non-BMP key ordering, which no published vector covers.
- **Security suite S1–S21** under `tests/security/`, with spec IDs in the test
  names.
- **Integration suite I1–I19** under `tests/integration/`, likewise ID-tagged,
  skipping cleanly without sandbox credentials.

### Fixed — webhook verification failed open

- `ToleranceSeconds: 0` **now means zero seconds of tolerance**, the strictest
  setting. It previously disabled the freshness check entirely, so a developer
  hardening a handler by reaching for `0` silently got no replay protection at
  all, with verification still returning `true`. Opting out now requires the
  explicit `DisableFreshnessCheck` sentinel. Any other negative value is
  rejected.
- `VerifyWebhookInput.Payload` is `[]byte` rather than `string`, hashed exactly
  as supplied.
- `VerifyWebhookSignature` is now a **package-level function**. It previously
  required constructing a client, and therefore API credentials, to check a
  webhook that is signed with an entirely different secret.

### Added — the missing half of the spec

- **`PaymentInstance` pubsub**: `On`, `Once` and `Off` over the `processing`,
  `success`, `failed`, `cancelled` and `error` events. The event types existed
  as declarations before but nothing emitted them. A handler registered after
  the call returned still receives events that already fired.
- **`initialError`**: a server-side initiation failure now surfaces as an
  `error` event on the instance rather than as a returned error. The returned
  error is reserved for client-side validation.
- **Poll interval backoff**: base interval for two minutes, then doubling every
  two minutes, capped at 15s, with jitter on every interval.
- **Delayed payments**: `Config.OnDelayed` selects whether to keep waiting or
  resolve with the still-pending record.
- **`ListTransactions` and `GetTransactionsByTag`**.
- **`PaymentInstance.Close`** stops polling. An abandoned instance previously
  polled for the life of the process.
- **Client cache** keyed on API key, secret hash and base URL, guarded by a
  mutex. Rotating the secret yields a fresh client; `Config.Force` bypasses it.
- `Config.HTTPClient`, `Config.MaxResponseBytes`, `Config.Force`,
  `Config.OnDelayed`.
- Status `on_hold`, `Transaction.StatusText`, `Transaction.Delayed`,
  `Transaction.Tags`, `StatusResponse.StatusText`, `StatusResponse.Delayed`,
  `Tags` on collect and payout inputs, `TransactionSummary`, `WebhookPayload`,
  `WebhookTransactionSnapshot`, `WebhookEventType`.

### Breaking

- **The `reference` is now a UUID.** It was validated as 13-15 characters and
  auto-generated as 15 hex characters, which the backend rejects outright with
  `reference must be a valid UUID`. Every collection and payout therefore failed
  against a live backend, and because a create call reports a server rejection
  on the instance rather than returning it, the failure surfaced downstream as a
  `not_found` on the follow-up lookup rather than at the call that caused it.
  References are now validated as UUIDs (any version) and generated as v4,
  matching the backend and the TypeScript reference SDK. Merchant order ids in
  another format must be converted to a UUID, or the generated reference kept
  alongside the order. Applies everywhere a reference is accepted, including
  `CreateInvoiceInput.MerchantReference`. Note that spec `operations.md` still
  documents the old 13-15 rule; it is being corrected separately.
- **`SDKError` is now importable.** It lived in `internal/core`, so no consumer
  could `errors.As` it — while the README and package docs both instructed
  exactly that, with a snippet that could not compile. It now lives in `types`
  and is re-exported as `nylonpay.SDKError`, alongside a typed `ErrorCategory`
  and its constants.
- Inputs renamed to the spec's names: `CollectPaymentPayload` →
  `CollectPaymentInput`, `MakePayoutPayload` → `MakePayoutInput`,
  `CreateInvoicePayload` → `CreateInvoiceInput`.
- **`CreateInvoiceInput` reshaped.** It was missing the required
  `CustomerEmail` and carried a `RedirectURL` that is not part of the contract,
  so no invoice call could succeed. Now: `CustomerEmail` (required),
  `CustomerName`, `CustomerPhone`, `DueDate`, `MerchantReference`, `Tags`.
- **`InvoiceResponse` corrected** to `{ID, InvoiceNumber, PaymentLink, Amount,
  Currency, Status}`. The previous `{ID, Url, Token, ExpiresAt, Status}` matched
  nothing on the wire.
- `GetStatus` and `VerifyPhone` take input structs rather than bare strings;
  `VerifyPhone` had no way to pass `Purpose` at all.
- **Hooks reshaped** to `Hook[T]{Enabled, Fn, OnError}` with a per-hook required
  `OnError`. A `before*` hook returning nil now leaves the input unchanged; it
  previously produced a nil payload that panicked the transport with
  "assignment to entry in nil map". A hook's output is re-validated before it is
  sent.
- `after*` hooks receive `HookResult` and `AfterHookInput`, the latter carrying
  both the final wire payload and the untouched original input.
- **Polling no longer caps by default.** `MaxPollDuration` and
  `MaxPollAttempts` defaulted to 5 minutes and 150 attempts; both now default to
  unset, meaning poll until terminal, per spec v1.5.
- `PaymentInstance.Status()` returns `TransactionStatus` rather than `string`.
- Metadata fields are `map[string]string` rather than `*map[string]string`.
- `Config.MaxRetries` accepts a negative value to mean "no retries", since zero
  is indistinguishable from unset.

### Fixed — correctness and concurrency

- The terminal-state transaction fetch no longer holds the instance's write lock
  across an HTTP round-trip, which blocked every accessor for its duration.
- Polling honours the caller's context; it previously used a hardcoded
  `context.Background()` with a 10s timeout, so cancelling the context passed to
  `CollectPayment` did nothing.
- Retry backoff is interruptible, and a cancelled request is no longer retried
  or reported as a timeout.
- Lifecycle events deduplicate by event rather than by raw status, so a
  `pending → processing` flap no longer re-fires `processing`.
- Error messages are humanized: no `nonce`, `HMAC`, signature verification,
  internal field names or raw error dumps reach a merchant. The machine-readable
  signal is `Category`.
- Reference length is counted in characters, not bytes.
- Panics in event handlers and hooks are contained.

### Security

- `tests/integration/.env`, which held live-format API credentials, is no longer
  tracked, and a `.gitignore` has been added. **Those credentials are in the git
  history and must be rotated.**
