// Package nylonpay is the official Go SDK for the Nylon Pay payment platform.
//
// It covers collecting payments, making payouts, verifying phone numbers,
// creating hosted invoices, looking up transactions, and verifying webhooks.
// Request signing, nonce generation, response verification, retries and polling
// are handled inside the transport; you never touch a crypto primitive.
//
// # Getting started
//
// Build one client and reuse it. Construction validates your credentials
// eagerly, so a bad key fails immediately rather than on the first payment:
//
//	client, err := nylonpay.NewClient(nylonpay.Config{
//	    APIKey:    os.Getenv("NYLONPAY_API_KEY"),    // npk_...
//	    APISecret: os.Getenv("NYLONPAY_API_SECRET"), // nps_...
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// Test and live mode follow from the key itself; there is no environment
// setting. Calling NewClient again with the same credentials returns the same
// client unless you pass Config.Force.
//
// # Collecting a payment
//
// The blocking form suits scripts and request handlers that can wait:
//
//	tx, err := client.CollectPaymentAndResolve(ctx, nylonpay.CollectPaymentInput{
//	    Amount:      10000, // smallest currency unit; minimum 500 UGX
//	    Currency:    nylonpay.UGX,
//	    Description: "Order #123",
//	    Customer: nylonpay.Customer{
//	        Name:        "Jane Doe",
//	        PhoneNumber: "0771234567", // normalized for you
//	    },
//	})
//
// The event-driven form suits long-lived servers. It returns immediately with
// an instance that polls in the background:
//
//	instance, err := client.CollectPayment(ctx, input)
//	if err != nil {
//	    return err // your input was invalid
//	}
//	defer instance.Close()
//
//	instance.
//	    On(nylonpay.PaymentEventProcessing, func(e nylonpay.EventData) {
//	        log.Printf("in flight: %s", e.Reference)
//	    }).
//	    On(nylonpay.PaymentEventSuccess, func(e nylonpay.EventData) {
//	        fulfill(e.Transaction)
//	    }).
//	    On(nylonpay.PaymentEventError, func(e nylonpay.EventData) {
//	        log.Printf("failed to start: %v (%s)", e.Err, e.Category)
//	    })
//
// Note where each kind of failure arrives. The error returned by CollectPayment
// is only ever about your input, something you can fix in code. A rejection by
// the server (bad key, provider refusal, network trouble) means no transaction
// was created, and it reaches you as an "error" event on the instance.
//
// Close the instance when you abandon one without waiting; otherwise it keeps
// polling for the life of the process.
//
// # References and idempotency
//
// The Reference is the transaction's identity and the only idempotency
// mechanism. Reusing one replays the existing transaction instead of charging
// again, which is what makes retrying a network failure safe. A fresh reference
// always starts a fresh payment.
//
// A supplied reference must be a UUID; omit it and one is generated. If your
// own order ids are in another format, derive a UUID from yours or keep the
// generated reference alongside your order.
//
// # Error handling
//
// Every error is an *SDKError carrying a Category from a fixed taxonomy.
// Branch on the category, never on the message text or an HTTP status:
//
//	tx, err := client.CollectPaymentAndResolve(ctx, input)
//	if err != nil {
//	    var sdkErr *nylonpay.SDKError
//	    if errors.As(err, &sdkErr) {
//	        switch sdkErr.Category {
//	        case nylonpay.CategoryDuplicate:
//	            // that reference belongs to another account; use a new one
//	        case nylonpay.CategoryRateLimit:
//	            // back off before trying again
//	        case nylonpay.CategoryProvider:
//	            // the payment itself failed; tx still holds the record
//	        }
//	    }
//	}
//
// # Webhooks
//
// Verification needs no client, only your webhook secret, which is a separate
// credential from the API secret:
//
//	body, _ := io.ReadAll(r.Body)
//	if !nylonpay.VerifyWebhookSignature(nylonpay.VerifyWebhookInput{
//	    Payload:   body, // the exact bytes received
//	    Signature: r.Header.Get("x-nylon-signature"),
//	    Secret:    webhookSecret,
//	}) {
//	    http.Error(w, "invalid signature", http.StatusUnauthorized)
//	    return
//	}
//
// Verification covers both authenticity and freshness, so a captured delivery
// replayed later is rejected. Pass the raw bytes: parsing and re-encoding the
// body changes key order and whitespace, and a genuine delivery would then fail
// its own signature.
//
// # Concurrency
//
// A client holds no state between calls and is safe for concurrent use. The
// only stateful object is a PaymentInstance, which is scoped to one transaction
// and safe to use from multiple goroutines.
//
// This package implements the Nylon Pay SDK specification, which is the
// cross-language contract all Nylon Pay SDKs share.
package nylonpay
