package fez

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

func esc(s string) string { return url.PathEscape(s) }

// ---- Auth ----

// Authenticate logs in and stores the bearer token and secret key on the client.
func (c *Client) Authenticate(ctx context.Context, userID, password string) (*AuthResponse, error) {
	var r AuthResponse
	err := c.do(ctx, http.MethodPost, "/user/authenticate",
		map[string]string{"user_id": userID, "password": password}, &r, false)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.token, c.secretKey, c.expires, c.userID = r.AuthDetails.AuthToken, r.OrgDetails.SecretKey, r.AuthDetails.ExpireToken, r.UserDetails.UserID
	c.mu.Unlock()
	return &r, nil
}

// Logout invalidates the session. userID is the internal id returned by
// Authenticate (UserDetails.UserID, e.g. "G-xxxx-xxxx"), NOT the login
// identifier you authenticated with. Pass "" to use the one stored by Authenticate.
func (c *Client) Logout(ctx context.Context, userID string) error {
	if userID == "" {
		c.mu.RLock()
		userID = c.userID
		c.mu.RUnlock()
	}
	return c.do(ctx, http.MethodPost, "/user/logout", map[string]string{"user_id": userID}, nil, false)
}

// ChangePassword: userID is the internal id (see Logout); "" uses the stored one.
func (c *Client) ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error {
	if userID == "" {
		c.mu.RLock()
		userID = c.userID
		c.mu.RUnlock()
	}
	return c.do(ctx, http.MethodPost, "/user/changePassword", map[string]string{
		"user_id": userID, "oldPassword": oldPassword, "newPassword": newPassword}, nil, false)
}

// ---- Orders ----

func (c *Client) CreateOrders(ctx context.Context, orders []CreateOrder) (*CreateOrdersResponse, error) {
	var r CreateOrdersResponse
	return c.keepBody(&r, c.do(ctx, http.MethodPost, "/order", orders, &r, false))
}

// UpdateOrder updates one order. Verified against the sandbox: PUT /order takes
// a single JSON object (the docs' array form is rejected with "order no field is required").
func (c *Client) UpdateOrder(ctx context.Context, o UpdateOrder) error {
	return c.do(ctx, http.MethodPut, "/order", o, nil, false)
}

// UpdateOrderKeepWaybill updates an order by its Fez order number and makes sure
// currentWaybill stays attached to it. Verified against the sandbox: an update
// without a waybill clears it, and the unchanged waybill cannot be sent in the
// same request, so this sends the field update first and then re-attaches it.
//   - o.WaybillNumber set: a single request that changes the waybill to that value.
//   - currentWaybill empty: a plain update.
//
// Between the two requests the waybill is briefly unset, so a failure of the
// second leaves the order without a waybill; the error says so.
func (c *Client) UpdateOrderKeepWaybill(ctx context.Context, o UpdateOrder, currentWaybill string) error {
	if o.WaybillNumber != "" || currentWaybill == "" {
		return c.UpdateOrder(ctx, o)
	}
	if err := c.UpdateOrder(ctx, o); err != nil {
		return err
	}
	if err := c.UpdateOrder(ctx, UpdateOrder{OrderNo: o.OrderNo, WaybillNumber: currentWaybill}); err != nil {
		return fmt.Errorf("fez: order %s updated but re-attaching waybill %q failed: %w", o.OrderNo, currentWaybill, err)
	}
	return nil
}

// UpdateOrders updates several orders with one request each (the API has no
// working batch form). It always attempts every order; Response maps orderNo to
// "Order Successfully Updated" or the error text. The returned error is non-nil
// if any order failed.
func (c *Client) UpdateOrders(ctx context.Context, orders []UpdateOrder) (*UpdateOrdersResponse, error) {
	r := &UpdateOrdersResponse{Envelope: Envelope{Status: "Success", Description: "Update Batch Request Processed"}, Response: map[string]string{}}
	var failed int
	for _, o := range orders {
		if err := c.UpdateOrder(ctx, o); err != nil {
			failed++
			r.Response[o.OrderNo] = err.Error()
			continue
		}
		r.Response[o.OrderNo] = "Order Successfully Updated"
	}
	if failed > 0 {
		r.Status = "Error"
		return r, fmt.Errorf("fez: %d of %d order updates failed", failed, len(orders))
	}
	return r, nil
}

func (c *Client) DeleteOrder(ctx context.Context, orderNo string) error {
	return c.do(ctx, http.MethodDelete, "/order", map[string]string{"orderNo": orderNo}, nil, false)
}

// CancelOrder cancels (and refunds, if paid) an order. reason max 255 chars.
func (c *Client) CancelOrder(ctx context.Context, orderNo, reason string) error {
	return c.do(ctx, http.MethodPost, "/order/cancel",
		map[string]string{"orderNo": orderNo, "reason": reason}, nil, false)
}

func (c *Client) GetOrder(ctx context.Context, orderNo string) (*GetOrderResponse, error) {
	var r GetOrderResponse
	return &r, c.do(ctx, http.MethodGet, "/orders/"+esc(orderNo), nil, &r, false)
}

func (c *Client) ManifestURL(ctx context.Context, orderNo string) (string, error) {
	var r ManifestResponse
	err := c.do(ctx, http.MethodGet, "/orders/"+esc(orderNo)+"/manifest-url", nil, &r, false)
	return r.Data.URL, err
}

func (c *Client) SearchOrders(ctx context.Context, q SearchOrders) (*SearchOrdersResponse, error) {
	if q.Page == 0 {
		q.Page = 1
	}
	var r SearchOrdersResponse
	return &r, c.do(ctx, http.MethodPost, "/orders/search", q, &r, false)
}

// SearchByWaybill finds an order by your own waybill number.
func (c *Client) SearchByWaybill(ctx context.Context, waybill string) (*WaybillSearchResponse, error) {
	var r WaybillSearchResponse
	return &r, c.do(ctx, http.MethodGet, "/orders/search/"+esc(waybill), nil, &r, false)
}

// OrderStats calls /orders/statsWithDateRange. endDate may be empty.
func (c *Client) OrderStats(ctx context.Context, startDate, endDate string) (*StatsResponse, error) {
	body := map[string]string{"startDate": startDate}
	if endDate != "" {
		body["endDate"] = endDate
	}
	var r StatsResponse
	return &r, c.do(ctx, http.MethodPost, "/orders/statsWithDateRange", body, &r, false)
}

func (c *Client) TrackOrder(ctx context.Context, orderNo string) (*TrackResponse, error) {
	var r TrackResponse
	return &r, c.do(ctx, http.MethodGet, "/order/track/"+esc(orderNo), nil, &r, false)
}

func (c *Client) DeliveryCost(ctx context.Context, req CostRequest) (*CostResponse, error) {
	var r CostResponse
	return &r, c.do(ctx, http.MethodPost, "/order/cost", req, &r, false)
}

// DeliveryTimeEstimate: the docs show a POST whose parameters appear in both
// the query string and body, so both are sent.
func (c *Client) DeliveryTimeEstimate(ctx context.Context, req DeliveryTimeEstimateRequest) (*DeliveryTimeEstimateResponse, error) {
	var r DeliveryTimeEstimateResponse
	return &r, c.do(ctx, http.MethodPost, "/delivery-time-estimate", req, &r, true)
}

func (c *Client) States(ctx context.Context) ([]State, error) {
	var r StatesResponse
	err := c.do(ctx, http.MethodGet, "/states", nil, &r, false)
	return r.States, err
}

func (c *Client) Hubs(ctx context.Context, stateID int) ([]Hub, error) {
	var r HubsResponse
	err := c.do(ctx, http.MethodGet, "/hubs/"+itoa(stateID), nil, &r, false)
	return r.Hubs, err
}

// ---- International ----

func (c *Client) CreateExportOrders(ctx context.Context, orders []ExportOrder) (*CreateOrdersResponse, error) {
	var r CreateOrdersResponse
	return c.keepBody(&r, c.do(ctx, http.MethodPost, "/orders/export", orders, &r, false))
}

func (c *Client) CreateImportOrders(ctx context.Context, orders []ImportOrder) (*CreateOrdersResponse, error) {
	var r CreateOrdersResponse
	return c.keepBody(&r, c.do(ctx, http.MethodPost, "/orders/import", orders, &r, false))
}

// ExportDeliveryCost: docs list GET with a body (their sample uses POST).
// Params are sent as query AND body to satisfy either server behaviour.
func (c *Client) ExportDeliveryCost(ctx context.Context, req ExportCostRequest) (*ExportCostResponse, error) {
	var r ExportCostResponse
	return &r, c.do(ctx, http.MethodGet, "/orders/export-price", req, &r, true)
}

func (c *Client) ImportDeliveryCost(ctx context.Context, req ImportCostRequest) (*ImportCostResponse, error) {
	var r ImportCostResponse
	return &r, c.do(ctx, http.MethodPost, "/orders/import-price", req, &r, false)
}

func (c *Client) ExportLocations(ctx context.Context) (*ExportLocationsResponse, error) {
	var r ExportLocationsResponse
	return &r, c.do(ctx, http.MethodGet, "/orders/export-locations", nil, &r, false)
}

func (c *Client) ImportLocations(ctx context.Context) ([]ImportLocation, error) {
	var r ImportLocationsResponse
	err := c.do(ctx, http.MethodGet, "/orders/import-locations", nil, &r, false)
	return r.Data.ImportLocations, err
}

func (c *Client) ImportItemCategories(ctx context.Context) ([]IDName, error) {
	var r ItemCategoriesResponse
	err := c.do(ctx, http.MethodGet, "/orders/item-categories", nil, &r, false)
	return r.Data.Categories, err
}

// ---- Lockers ----

func (c *Client) Lockers(ctx context.Context, state string) (*LockersResponse, error) {
	var r LockersResponse
	return &r, c.do(ctx, http.MethodGet, "/Lockers/"+esc(state), nil, &r, false)
}

func (c *Client) LockerAvailable(ctx context.Context, lockerID string) (bool, error) {
	var r LockerAvailabilityResponse
	err := c.do(ctx, http.MethodGet, "/LockerAvailability/"+esc(lockerID), nil, &r, false)
	return r.Available, err
}

// ---- Users ----

func (c *Client) CreateUser(ctx context.Context, u CreateUser) (string, error) {
	var r CreateUserResponse
	err := c.do(ctx, http.MethodPost, "/user", u, &r, false)
	return r.UserID, err
}

// DeleteUser takes the user's numeric id from Users() (User.ID), per the docs.
func (c *Client) DeleteUser(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/user/"+esc(id)+"/delete", nil, nil, false)
}

func (c *Client) Users(ctx context.Context) ([]User, error) {
	var r UsersResponse
	err := c.do(ctx, http.MethodGet, "/users", nil, &r, false)
	return r.Users, err
}

// ---- Webhooks ----

func (c *Client) RegisterWebhook(ctx context.Context, webhookURL string) (*RegisterWebhookResponse, error) {
	var r RegisterWebhookResponse
	return &r, c.do(ctx, http.MethodPost, "/webhooks/store", map[string]string{"webhook": webhookURL}, &r, false)
}

// keepBody: on an APIError from a create call the API still returns per-order
// results in orderNos (e.g. "Insufficient wallet balance..."). Decode them into
// r so callers get both the error and the per-order messages.
func (c *Client) keepBody(r *CreateOrdersResponse, err error) (*CreateOrdersResponse, error) {
	if ae, ok := err.(*APIError); ok {
		_ = json.Unmarshal([]byte(ae.Body), r)
		return r, err
	}
	return r, err
}
