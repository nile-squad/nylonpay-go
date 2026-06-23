# nylonpay-go

Go SDK for the [Nile Squad](https://nilesquad.com) payment platform.

## Installation

```sh
go get github.com/nile-squad/nylonpay-go
```

Requires Go 1.21+.

## Quick start

```go
import nylonpay "github.com/nile-squad/nylonpay-go"

client, err := nylonpay.NewClient(nylonpay.Config{
    APIKey:    "npk_...",
    APISecret: "nps_...",
})
if err != nil {
    log.Fatal(err)
}
```

Create the client once and reuse it. It is safe for concurrent use.

## Operations

### Collect a payment

```go
// Block until the payment settles.
tx, err := client.CollectPaymentAndResolve(ctx, nylonpay.CollectPaymentPayload{
    Amount:      10000,
    Currency:    nylonpay.UGX,
    Description: "Order #8821",
    Customer: nylonpay.Customer{
        Name:        "Jane Doe",
        PhoneNumber: "0771234567",
    },
})

// Or get a handle and wait when convenient.
instance, err := client.CollectPayment(ctx, payload)
// ...do other work...
tx, err = instance.Wait(ctx)
```

Minimum amount: **500 UGX**.

### Make a payout

```go
tx, err := client.MakePayoutAndResolve(ctx, nylonpay.MakePayoutPayload{
    Amount:      50000,
    Currency:    nylonpay.UGX,
    Description: "Vendor settlement",
    Customer: nylonpay.Customer{
        Name:        "John Smith",
        PhoneNumber: "0701234567",
    },
    Destination: nylonpay.Destination{
        AccountHolderName: "John Smith",
        AccountNumber:     "0701234567",
    },
})
```

Minimum amount: **5000 UGX**.

### Get transaction status

```go
status, err := client.GetStatus(ctx, "ref1234567890abc")

tx, err := client.GetTransaction(ctx, nylonpay.GetTransactionInput{
    Reference: "ref1234567890abc",
})
```

### Verify a phone number

```go
info, err := client.VerifyPhone(ctx, "0771234567")
fmt.Println(info.CustomerName, info.Verified)
```

### Create an invoice

```go
invoice, err := client.CreateInvoice(ctx, nylonpay.CreateInvoicePayload{
    Amount:      25000,
    Currency:    nylonpay.UGX,
    Description: "Invoice #44",
})
fmt.Println(invoice.Url)
```

Invoice creation is available in live mode only.

### Verify a webhook

```go
ok := client.VerifyWebhookSignature(nylonpay.VerifyWebhookInput{
    Payload:   string(body),
    Signature: r.Header.Get("X-Nylon-Signature"),
    Secret:    "your-webhook-secret",
})
```

By default, payloads older than 5 minutes are rejected. Override with `ToleranceSeconds`.

## References

Supply your own reference (13–15 characters) or leave it empty for an auto-generated one:

```go
nylonpay.CollectPaymentPayload{
    Reference: "ord20240601001",
    // ...
}
```

The same reference on a repeated request replays the original transaction (idempotency).

## Error handling

All errors are `*core.SDKError`:

```go
import "github.com/nile-squad/nylonpay-go/internal/core"

tx, err := client.CollectPaymentAndResolve(ctx, payload)
if err != nil {
    var sdkErr *core.SDKError
    if errors.As(err, &sdkErr) {
        fmt.Println(sdkErr.Category) // "validation", "auth", "provider", ...
        fmt.Println(sdkErr.Message)
    }
}
```

| Category | Origin |
|----------|--------|
| `auth` | Bad API key or secret |
| `validation` | Invalid request input |
| `provider` | Downstream payment provider |
| `not_found` | Reference does not exist |
| `duplicate` | Reference belongs to another account |
| `rate_limit` | Too many requests |
| `limit` | Account limit reached |
| `account` | Account configuration issue |
| `network` | Connection failure |
| `timeout` | Request or polling timeout |
| `internal` | Unexpected server error |

## Hooks

Hooks run around collection and payout operations. A panicking hook is recovered silently; set `OnError` to be notified.

```go
client, _ := nylonpay.NewClient(nylonpay.Config{
    APIKey:    "npk_...",
    APISecret: "nps_...",
    Hooks: &nylonpay.Hooks{
        BeforeCollect: func(p *nylonpay.CollectPaymentPayload) *nylonpay.CollectPaymentPayload {
            p.Description = strings.TrimSpace(p.Description)
            return p
        },
        AfterCollect: func(p *nylonpay.CollectPaymentPayload, ref, status string, err error) {
            log.Printf("collect ref=%s status=%s err=%v", ref, status, err)
        },
        OnError: func(hook string, err error) {
            log.Printf("hook %s panicked: %v", hook, err)
        },
    },
})
```

## Configuration

| Field | Default | Description |
|-------|---------|-------------|
| `APIKey` | — | Required. Must start with `npk_`. |
| `APISecret` | — | Required. Must start with `nps_`. |
| `BaseURL` | `https://api.nylonpay.nilesquad.com/api/services` | Override for testing. |
| `Timeout` | `30s` | Per-request timeout. |
| `MaxRetries` | `3` | Retries on transient errors (5xx, 408, 429). |
| `MaxPollInterval` | `2s` | Interval between status polls. |
| `MaxPollDuration` | `5m` | Hard ceiling on total poll time. |
| `MaxPollAttempts` | `150` | Maximum number of poll attempts. |

## Testing

```sh
# Unit tests
go test ./...

# Integration tests (requires sandbox credentials)
NYLONPAY_API_KEY=npk_... \
NYLONPAY_API_SECRET=nps_... \
NYLONPAY_TEST_PHONE=07... \
  go test ./tests/integration/ -tags integration -v
```
