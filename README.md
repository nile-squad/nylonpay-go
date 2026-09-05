# nylonpay-go

Official Go SDK for the [Nylon Pay](https://nylonpay.nilesquad.com) payment platform.

Collect payments, make payouts, verify phone numbers, create hosted invoices,
look up transactions, and verify webhooks. Request signing, response
verification, retries and status polling are handled inside the SDK.

This package implements the Nylon Pay SDK specification, the cross-language
contract every Nylon Pay SDK shares, and ships its canonical signing
conformance vectors (V1–V7), security suite (S1–S21) and integration suite
(I1–I19).

## Installation

```bash
go get github.com/nile-squad/nylonpay-go
```

Requires Go 1.25 or later. No third-party dependencies.

## Quick start

```go
client, err := nylonpay.NewClient(nylonpay.Config{
    APIKey:    os.Getenv("NYLONPAY_API_KEY"),    // npk_...
    APISecret: os.Getenv("NYLONPAY_API_SECRET"), // nps_...
})
if err != nil {
    log.Fatal(err)
}

tx, err := client.CollectPaymentAndResolve(ctx, nylonpay.CollectPaymentInput{
    Amount:      10000, // smallest currency unit; minimum 500 UGX
    Currency:    nylonpay.UGX,
    Description: "Order #123",
    Customer: nylonpay.Customer{
        Name:        "Jane Doe",
        PhoneNumber: "0771234567",
    },
})
```

Test or live mode follows from the API key. There is no environment setting.

Construction validates credentials eagerly, and caches one client per
key + secret + base URL, so calling `NewClient` again with the same credentials
hands back the same client. Pass `Force: true` for a fresh one.

## Operations

| Operation | Shape | Returns |
|---|---|---|
| `CollectPayment` | async | `*PaymentInstance` |
| `CollectPaymentAndResolve` | blocking | `*Transaction` |
| `MakePayout` | async | `*PaymentInstance` |
| `MakePayoutAndResolve` | blocking | `*Transaction` |
| `GetStatus` | sync | `*StatusResponse` |
| `GetTransaction` | sync | `*Transaction` |
| `ListTransactions` | sync | `*ListTransactionsResponse` |
| `GetTransactionsByTag` | sync | `*ListTransactionsResponse` |
| `VerifyPhone` | sync | `*PhoneVerification` |
| `CreateInvoice` | sync | `*InvoiceResponse` |
| `VerifyWebhookSignature` | utility | `bool` |

### Tracking a payment with events

```go
instance, err := client.CollectPayment(ctx, input)
if err != nil {
    return err // your input was invalid
}
defer instance.Close()

instance.
    On(nylonpay.PaymentEventProcessing, func(e nylonpay.EventData) {
        log.Printf("in flight: %s", e.Reference)
    }).
    On(nylonpay.PaymentEventSuccess, func(e nylonpay.EventData) {
        fulfill(e.Transaction)
    }).
    On(nylonpay.PaymentEventError, func(e nylonpay.EventData) {
        log.Printf("could not start: %v (%s)", e.Err, e.Category)
    })

tx, err := instance.Wait(ctx)
```

Events are `processing`, `success`, `failed`, `cancelled` and `error`. Each
fires at most once. `processing` covers `pending`, `processing` and `on_hold`
alike — one lifecycle moment to a merchant — and always fires before a terminal
event, even for a payment that settles between polls.

Registering a handler after the call returned is safe: an event that has already
fired is delivered to the new handler immediately.

`Close` stops polling. Call it on any instance you abandon without waiting,
otherwise it keeps polling for the life of the process.

**Where failures arrive.** The error returned by `CollectPayment` and
`MakePayout` is only ever about your input. A rejection by the server means no
transaction was created, and it reaches you as an `error` event on the instance.

### Polling behaviour

By default an instance polls until the transaction settles. The interval starts
at 2s with jitter, doubles every two minutes, and is capped at 15s. Set
`MaxPollDuration` or `MaxPollAttempts` to bound the wait.

A payment in flight for more than about three minutes is flagged `Delayed`.
`OnDelayed: nylonpay.OnDelayedReturn` resolves with the still-pending record so
you can rely on webhooks instead of waiting.

### References and idempotency

The reference is the transaction's identity and the only idempotency mechanism.
Reusing one replays the existing transaction instead of charging again, which is
what makes retrying a network failure safe; a fresh reference always starts a
fresh payment.

A supplied reference must be **13–15 characters**. Omit it and one is generated.
The usual mistake is passing a 36-character UUID order id — hash or truncate it
first.

### Webhooks

```go
body, _ := io.ReadAll(r.Body)

if !nylonpay.VerifyWebhookSignature(nylonpay.VerifyWebhookInput{
    Payload:   body, // the exact bytes received
    Signature: r.Header.Get("x-nylon-signature"),
    Secret:    webhookSecret,
}) {
    http.Error(w, "invalid signature", http.StatusUnauthorized)
    return
}

var delivery nylonpay.WebhookPayload
json.Unmarshal(body, &delivery)
```

Pass the **raw bytes**. Parsing and re-encoding changes key order and
whitespace, so a genuine delivery would fail its own signature.

The webhook secret is a **separate credential** from the API secret. Using the
API secret here makes every webhook fail.

Verification covers authenticity *and* freshness, so a captured delivery
replayed later is rejected. `ToleranceSeconds` defaults to 300.
`ToleranceSeconds: 0` means zero seconds — the strictest possible setting, not
an off switch. To opt out, pass `nylonpay.DisableFreshnessCheck`.

Delivery is at-least-once, so deduplicate on `DeliveryID` or `Reference`.

## Error handling

Every error is an `*SDKError` with a `Category` from a fixed taxonomy. Branch on
the category, never on message text or an HTTP status — the backend answers 200
for success and 400 for every failure regardless of cause.

```go
var sdkErr *nylonpay.SDKError
if errors.As(err, &sdkErr) {
    switch sdkErr.Category {
    case nylonpay.CategoryDuplicate:
        // that reference belongs to another account; use a new one
    case nylonpay.CategoryRateLimit:
        // back off before retrying
    case nylonpay.CategoryProvider:
        // the payment itself failed
    }
}
```

`auth`, `validation`, `limit`, `rate_limit`, `account`, `provider`, `duplicate`,
`not_found`, `internal`, `network`, `timeout`.

## Hooks

Hooks observe or enrich payment calls. Each wraps a handler with a required
`OnError`, so a bug in your hook can neither crash a payment nor be swallowed.

```go
client, _ := nylonpay.NewClient(nylonpay.Config{
    APIKey:    key,
    APISecret: secret,
    Hooks: &nylonpay.Hooks{
        BeforeCollect: &nylonpay.Hook[func(nylonpay.CollectPaymentInput) *nylonpay.CollectPaymentInput]{
            Fn: func(in nylonpay.CollectPaymentInput) *nylonpay.CollectPaymentInput {
                in.Metadata["tenant"] = currentTenant()
                return &in // return nil to leave the input unchanged
            },
            OnError: func(err error) { log.Printf("hook failed: %v", err) },
        },
        AfterCollect: &nylonpay.Hook[func(nylonpay.HookResult, nylonpay.AfterHookInput[nylonpay.CollectPaymentInput])]{
            Fn: func(res nylonpay.HookResult, in nylonpay.AfterHookInput[nylonpay.CollectPaymentInput]) {
                // in.Input is the final wire payload; in.Raw is what you passed
                audit(res.Reference, res.Status, res.Err)
            },
            OnError: func(err error) { log.Printf("hook failed: %v", err) },
        },
    },
})
```

A `before*` hook's output is re-run through the full validation and
normalization suite, so it cannot smuggle a bad reference or a sub-minimum
amount past the checks. An `after*` hook runs whether the call succeeded or not.
Set `Enabled` to `false` to switch one off without removing it.

## Configuration

| Field | Default | Notes |
|---|---|---|
| `APIKey` | — | required, `npk_` prefix |
| `APISecret` | — | required, `nps_` prefix |
| `BaseURL` | production endpoint | complete URL including path |
| `Timeout` | `30s` | per HTTP attempt |
| `MaxRetries` | `3` | pass a negative value to disable |
| `MaxPollInterval` | `2s` | base poll interval, not a ceiling |
| `MaxPollDuration` | none | zero polls until terminal |
| `MaxPollAttempts` | none | zero polls until terminal |
| `OnDelayed` | `wait` | or `return` |
| `HTTPClient` | `http.Client` | substitute your own |
| `MaxResponseBytes` | `10 MB` | response body cap |
| `Force` | `false` | bypass the client cache |
| `Hooks` | none | see above |

Retries cover 408, 429, 500, 502, 503, 504 and network failures, with
exponential backoff. Other 4xx responses are returned immediately. Every attempt
is signed fresh while the body — and therefore the reference — stays constant,
so a retry replays rather than double-charges.

## Testing

The suites run through the [Makefile](./Makefile):

```bash
make test              # unit + security suites
make test-race         # with the race detector
make test-integration  # needs sandbox credentials
```

Every target sets `GO_ENV=testing`, which is what makes the integration suite
load `tests/integration/.env`. Calling `go test` directly skips that.

The signing conformance vectors are the gate. If they fail, every live request
will fail with an opaque `auth` error:

```bash
make test-crypto
```

Integration tests read `NYLONPAY_API_KEY`, `NYLONPAY_API_SECRET`,
`NYLONPAY_TEST_PHONE`, `NYLONPAY_BASE_URL` and `NYLONPAY_TEST_MODE` (see
`tests/integration/.env.example`) and skip cleanly when credentials are absent.

### Windows

Windows ships no `make`, so install one first:

```powershell
winget install ezwinports.make   # or: choco install make, scoop install make
```

Running the suites from WSL or Git Bash works too.

If you invoke `go test` directly instead, note that the `GO_ENV=testing go test`
form in the Makefile is POSIX shell syntax and does nothing in `cmd.exe` or
PowerShell. Set the variable separately:

```powershell
$env:GO_ENV = "testing"; go test ./... -v
```

```bat
set GO_ENV=testing && go test ./... -v
```

## License

See [LICENSE](./LICENSE).
