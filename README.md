# fez-logistics

Go client for the [Fez Delivery Business API](https://fez-delivery-co.gitbook.io/fezcorporate-api-docs) — auth, local & international orders, pricing, hubs/states, lockers, users and webhooks.

```go
import fez "github.com/kellslte/fez-logistics"

c := fez.New(fez.WithEnvironment(fez.Production)) // default is fez.Sandbox
_, err := c.Authenticate(ctx, userID, password) // stores token + secret-key
res, err := c.CreateOrders(ctx, []fez.CreateOrder{{
    RecipientName: "Femi", RecipientPhone: "0800...", RecipientAddress: "Idumota",
    RecipientState: "Lagos", UniqueID: "ord-1", BatchID: "b-1", ValueOfItem: "20000", Weight: 1,
}})
// res.OrderNos["ord-1"] -> Fez order number
```

Environment: `fez.Sandbox` (default, `https://apisandbox.fezdelivery.co/v1`) or `fez.Production` (`https://api.fezdelivery.co/v1`). From config: `env, err := fez.ParseEnvironment(os.Getenv("FEZ_ENV"))`. `WithBaseURL` overrides both. An unknown environment makes calls fail instead of falling back.

Webhooks: `c.RegisterWebhook(ctx, url)`, then serve `fez.WebhookHandler(secretKey, func(ev fez.WebhookEvent){...})`.

## Coverage (all 30 documented endpoints (plus the inbound webhook))

Auth: Authenticate, Logout, ChangePassword · Orders: CreateOrders, UpdateOrders, DeleteOrder, CancelOrder, GetOrder, ManifestURL, SearchOrders, SearchByWaybill, OrderStats, TrackOrder, DeliveryCost, DeliveryTimeEstimate, States, Hubs · International: CreateExportOrders, CreateImportOrders, ExportDeliveryCost, ImportDeliveryCost, ExportLocations, ImportLocations, ImportItemCategories · Lockers: Lockers, LockerAvailable · Users: CreateUser, DeleteUser, Users · Webhooks: RegisterWebhook, Sign/VerifyWebhook/WebhookHandler.

## Doc ambiguities (unverified against a live account)

- `ExportDeliveryCost` is documented as GET but sampled as POST; params go in query and body.
- Update's sample URL is `/order/multiple`; the endpoint header says `PUT /order` (used).
- Webhook signature is assumed HMAC-SHA256(secretKey, orderNo+status+timestamp).
- Import sample misspells `qunatity`; the field table says `quantity` (used).

## Releasing

`scripts/bump.sh patch|minor|major [--push]` runs vet+tests, updates `Version` in `version.go`, commits and tags `vX.Y.Z`. `fez.Version` is also sent in the `User-Agent`.
