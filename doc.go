// Package fez is a Go client for the Fez Delivery Business (corporate) API.
//
// Create a client, authenticate once, then call methods. Fez order numbers
// (e.g. "2W0V24112651") are the source of truth for every order operation.
//
//	c := fez.New(fez.WithEnvironment(fez.Sandbox)) // Sandbox is the default
//	if _, err := c.Authenticate(ctx, email, password); err != nil { ... }
//	res, err := c.CreateOrders(ctx, []fez.CreateOrder{{...}})
//
// # Behaviour verified against the real sandbox (differs from Fez's docs)
//
//   - PUT /order takes ONE order object, not an array; UpdateOrders therefore
//     sends one request per order.
//   - A successful update that omits WaybillNumber clears the order's waybill.
//     Re-sending the unchanged waybill in the same request fails ("already
//     taken"). Use UpdateOrderKeepWaybill.
//   - Logout and ChangePassword need the internal user id returned by
//     Authenticate (e.g. "G-xxxx-xxxx"), not the login email. Passing "" uses it.
//   - GET /orders/{orderNo} never returns the waybill number; keep your own copy.
//   - /states needs no credentials. Everything else needs the bearer token and
//     the secret-key header (both are set by Authenticate).
//   - recipientName must be at least 5 characters.
//   - Failed order creation returns HTTP 400 with per-order messages in
//     CreateOrdersResponse.OrderNos (e.g. "Insufficient wallet balance").
//   - Import location 1 (Niger) has no price in the sandbox.
//
// Webhook delivery and signature format are NOT verified against real Fez
// traffic; see Sign and VerifyWebhook.
//
// Full notes: https://github.com/kellslte/fez-logistics#readme
package fez
