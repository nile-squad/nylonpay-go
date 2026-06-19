// Package nylonpay is the official Go SDK for the Nylon Pay payment platform.
//
// Initialize a client once with your API credentials and reuse it for the
// lifetime of your application:
//
//	client, err := nylonpay.NewClient(nylonpay.Config{
//	    APIKey:    "npk_...",
//	    APISecret: "nps_...",
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// # Collecting a payment
//
//	tx, err := client.CollectPaymentAndResolve(ctx, nylonpay.CollectPaymentPayload{
//	    Amount:      10000,
//	    Currency:    nylonpay.UGX,
//	    Description: "Order #123",
//	    Customer: nylonpay.Customer{
//	        Name:        "Jane Doe",
//	        PhoneNumber: "0771234567",
//	    },
//	})
//
// # Async tracking
//
// Use CollectPayment (or MakePayout) to get a PaymentInstance and call Wait
// when you are ready to block:
//
//	instance, _ := client.CollectPayment(ctx, payload)
//	tx, err := instance.Wait(ctx)
//
// # Error handling
//
// All errors returned by the SDK are of type *core.SDKError and carry a
// machine-readable Category field ("validation", "auth", "network", etc.).
package nylonpay
