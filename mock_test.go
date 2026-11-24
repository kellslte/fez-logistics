package fez

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// mockFez is a stateful fake of the Fez API built from the public docs. It
// enforces headers, HTTP methods and the documented JSON keys, so it verifies
// what the SDK puts on the wire. It cannot prove the real server matches docs.
type mockFez struct {
	t      *testing.T
	mu     sync.Mutex
	orders map[string]map[string]any // orderNo -> fields
	status map[string]string
	n      int
	hook   string
	wallet bool // true = funded
}

func newMock(t *testing.T) (*mockFez, *Client) {
	m := &mockFez{t: t, orders: map[string]map[string]any{}, status: map[string]string{}, wallet: true}
	srv := httptest.NewServer(m)
	t.Cleanup(srv.Close)
	return m, New(WithBaseURL(srv.URL))
}

func reply(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
func ok(w http.ResponseWriter, extra map[string]any) {
	b := map[string]any{"status": "Success", "description": "ok"}
	for k, v := range extra {
		b[k] = v
	}
	reply(w, 200, b)
}
func fail(w http.ResponseWriter, code int, d string) {
	reply(w, code, map[string]any{"status": "Error", "description": d})
}

func (m *mockFez) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := r.URL.Path
	if p == "/user/authenticate" {
		var b map[string]string
		json.NewDecoder(r.Body).Decode(&b)
		if b["user_id"] != "me@x.com" || b["password"] != "pw" {
			fail(w, 401, "bad login")
			return
		}
		ok(w, map[string]any{
			"authDetails": map[string]any{"authToken": "TOK", "expireToken": "2030-01-01 00:00:00"},
			"userDetails": map[string]any{"userID": "G-1", "Full Name": "A", "Username": "Admin"},
			"orgDetails":  map[string]any{"secret-key": "SK", "Org Full Name": "Org"}})
		return
	}
	if p != "/states" { // docs: all else needs both headers
		if r.Header.Get("secret-key") != "SK" {
			fail(w, 401, "Organization Secret Key is Required")
			return
		}
		if r.Header.Get("Authorization") != "Bearer TOK" {
			fail(w, 401, "Invalid session")
			return
		}
	}
	var body map[string]any
	var arr []map[string]any
	raw := new(bytes.Buffer)
	raw.ReadFrom(r.Body)
	if raw.Len() > 0 {
		if raw.Bytes()[0] == '[' {
			json.Unmarshal(raw.Bytes(), &arr)
		} else {
			json.Unmarshal(raw.Bytes(), &body)
		}
	}
	want := func(method string) bool {
		if r.Method != method {
			fail(w, 405, "wrong method "+r.Method+" for "+p)
			m.t.Errorf("%s: method %s want %s", p, r.Method, method)
			return false
		}
		return true
	}
	need := func(src map[string]any, keys ...string) bool {
		for _, k := range keys {
			if _, ok := src[k]; !ok {
				m.t.Errorf("%s: missing JSON key %q in %s", p, k, raw)
				fail(w, 422, "The "+k+" field is required.")
				return false
			}
		}
		return true
	}
	switch {
	case p == "/user/logout" && want("POST"):
		if need(body, "user_id") {
			ok(w, nil)
		}
	case p == "/user/changePassword" && want("POST"):
		if need(body, "user_id", "oldPassword", "newPassword") {
			ok(w, nil)
		}
	case p == "/order" && r.Method == "POST":
		if !m.wallet {
			res := map[string]string{}
			for _, o := range arr {
				res[o["uniqueID"].(string)] = "Insufficient wallet balance."
			}
			reply(w, 400, map[string]any{"status": "Error", "description": "Error creating orders", "orderNos": res, "duplicateUniqueIds": []string{}})
			return
		}
		m.create(w, arr, "recipientAddress", "recipientState", "recipientName", "recipientPhone", "uniqueID", "BatchID", "valueOfItem", "weight")
	case p == "/orders/export" && want("POST"):
		m.create(w, arr, "recipientAddress", "recipientName", "recipientPhone", "uniqueID", "BatchID", "valueOfItem", "weight", "exportLocationId")
	case p == "/orders/import" && want("POST"):
		m.create(w, arr, "recipientAddress", "recipientState", "recipientName", "recipientPhone", "uniqueID", "BatchID", "valueOfItem", "weight", "importLocationId", "itemCategory", "businessName")
	case p == "/order" && r.Method == "PUT":
		// verified against the real sandbox: single object, not an array
		if !need(body, "orderNo") {
			return
		}
		no := body["orderNo"].(string)
		if _, e := m.orders[no]; !e {
			fail(w, 400, "Order not found")
			return
		}
		for k, v := range body {
			m.orders[no][k] = v
		}
		ok(w, map[string]any{"description": "Order Successfully Updated"})
	case p == "/order" && r.Method == "DELETE":
		if need(body, "orderNo") {
			delete(m.orders, body["orderNo"].(string))
			ok(w, nil)
		}
	case p == "/order/cancel" && want("POST"):
		if !need(body, "orderNo", "reason") {
			return
		}
		no := body["orderNo"].(string)
		if _, e := m.orders[no]; !e {
			fail(w, 442, "Order not found or does not belong to your organization.")
		} else if s := m.status[no]; s != "Pending Pick-Up" {
			fail(w, 400, "Invalid order status")
		} else {
			m.status[no] = "Cancelled"
			ok(w, nil)
		}
	case strings.HasPrefix(p, "/orders/search/") && want("GET"):
		wb := strings.TrimPrefix(p, "/orders/search/")
		for no, o := range m.orders {
			if o["waybillNumber"] == wb {
				ok(w, map[string]any{"data": map[string]any{"orderNumber": no, "waybillNumber": wb}})
				return
			}
		}
		fail(w, 404, "not found")
	case p == "/orders/search" && want("POST"):
		if !need(body, "startDate", "endDate", "page") {
			return
		}
		var rows []any
		for no, o := range m.orders {
			if f, has := body["orderNo"]; has && f != no {
				continue
			}
			rows = append(rows, map[string]any{"orderNo": no, "recipientName": o["recipientName"], "orderStatus": m.status[no], "cost": "900", "orderDate": "2026-01-01 00:00:00", "pickUpDate": "0000-00-00 00:00:00"})
		}
		ok(w, map[string]any{"orders": map[string]any{"current_page": 1, "data": rows, "last_page": 1, "next_page_url": nil, "per_page": 50, "total": len(rows), "from": 1, "to": len(rows)}})
	case p == "/orders/statsWithDateRange" && want("POST"):
		if need(body, "startDate") {
			ok(w, map[string]any{"orderDetails": []any{map[string]any{"orderNo": "X", "orderStatus": "Pending Pick-Up", "cost": "900"}}})
		}
	case strings.HasSuffix(p, "/manifest-url") && want("GET"):
		no := strings.TrimSuffix(strings.TrimPrefix(p, "/orders/"), "/manifest-url")
		if _, e := m.orders[no]; !e {
			fail(w, 404, "Manifest is not available for this order.")
		} else {
			ok(w, map[string]any{"data": map[string]any{"url": "https://files/" + no + ".pdf"}})
		}
	case strings.HasPrefix(p, "/orders/export-") || p == "/orders/import-price" || p == "/orders/item-categories" || p == "/orders/import-locations":
		m.intl(w, r, body)
	case strings.HasPrefix(p, "/orders/") && want("GET"):
		no := strings.TrimPrefix(p, "/orders/")
		o, e := m.orders[no]
		if !e {
			fail(w, 404, "not found")
			return
		}
		ok(w, map[string]any{"orderDetails": []any{map[string]any{"orderNo": no, "recipientName": o["recipientName"], "orderStatus": m.status[no], "cost": "3398.36", "weight": "2", "quantity": 1, "is_item_cod": 0, "cod_amount": nil, "insured": false}}})
	case strings.HasPrefix(p, "/order/track/") && want("GET"):
		no := strings.TrimPrefix(p, "/order/track/")
		if _, e := m.orders[no]; !e {
			fail(w, 404, "not found")
			return
		}
		ok(w, map[string]any{"order": map[string]any{"orderNo": no, "orderStatus": m.status[no], "proofOfDelivery": nil}, "history": []any{map[string]any{"orderStatus": "Picked-Up", "statusCreationDate": "2026-01-01 10:00:00", "statusDescription": "d"}}})
	case p == "/order/cost" && want("POST"):
		ok(w, map[string]any{"cost": map[string]any{"state": "Kano", "cost": 6181.25}, "Cost": map[string]any{"state": "Kano", "cost": 6181.25},
			"vat": map[string]any{"vatAmount": 431.25, "vatPercent": 7.5}, "surcharge": map[string]any{"hasSurcharge": true, "items": []any{map[string]any{"id": 1, "name": "Fuel", "surge_type": "FUEL", "rate": 10, "calculation_type": "percentage", "amount": 575}}, "totalAmount": 575}, "totalCost": 6756.25})
	case p == "/delivery-time-estimate" && want("POST"):
		if need(body, "delivery_type") {
			ok(w, map[string]any{"data": map[string]any{"eta": "2 - 5 day(s)"}})
		}
	case p == "/states":
		ok(w, map[string]any{"states": []any{map[string]any{"id": 1, "state": "Kano"}}})
	case strings.HasPrefix(p, "/hubs/") && want("GET"):
		ok(w, map[string]any{"hubs": []any{map[string]any{"id": 1, "name": "Lagoswa", "address": "Lagos"}}})
	case strings.HasPrefix(p, "/LockerAvailability/") && want("GET"):
		ok(w, map[string]any{"Available": true})
	case strings.HasPrefix(p, "/Lockers/") && want("GET"):
		ok(w, map[string]any{"Lockers": []any{map[string]any{"lockerID": "2100015346", "lockerAddress": "a"}}, "maxWeight": "5", "maxValueOfItem": "100000"})
	case p == "/user" && want("POST"):
		if need(body, "fullname", "email", "password", "role") {
			ok(w, map[string]any{"user_id": "G-new"})
		}
	case strings.HasPrefix(p, "/user/") && strings.HasSuffix(p, "/delete") && want("POST"):
		ok(w, nil)
	case p == "/users" && want("GET"):
		ok(w, map[string]any{"users": []any{map[string]any{"id": 71, "user_id": "G-new", "fullname": "N", "email": "n@x.com", "created_at": "2024-04-01T10:40:05.000000Z"}}})
	case p == "/webhooks/store" && want("POST"):
		if need(body, "webhook") {
			m.hook = body["webhook"].(string)
			ok(w, map[string]any{"data": map[string]any{"webhook": m.hook, "webhooks": []any{map[string]any{"url": m.hook, "type": "App\\Models\\Order", "is_active": 1}}}})
		}
	default:
		m.t.Errorf("unexpected request %s %s", r.Method, p)
		fail(w, 404, "no route")
	}
}

func (m *mockFez) create(w http.ResponseWriter, arr []map[string]any, req ...string) {
	res := map[string]string{}
	for _, o := range arr {
		for _, k := range req {
			if _, has := o[k]; !has {
				m.t.Errorf("create: missing JSON key %q", k)
				fail(w, 422, "The "+k+" field is required.")
				return
			}
		}
		m.n++
		no := "FZ" + strconv.Itoa(1000+m.n)
		m.orders[no], m.status[no] = o, "Pending Pick-Up"
		res[o["uniqueID"].(string)] = no
	}
	reply(w, 201, map[string]any{"status": "Success", "description": "Order Successfully Created", "orderNos": res})
}

func (m *mockFez) intl(w http.ResponseWriter, r *http.Request, body map[string]any) {
	switch r.URL.Path {
	case "/orders/export-locations":
		ok(w, map[string]any{"data": map[string]any{"exportLocations": []any{map[string]any{"id": 1, "name": "Canada"}}, "exportWeights": []any{map[string]any{"id": 1, "name": "0 - 2"}}}})
	case "/orders/import-locations":
		ok(w, map[string]any{"data": map[string]any{"importLocations": []any{map[string]any{"id": 1, "city": nil, "state": nil, "name": "Niger"}, map[string]any{"id": 2, "city": "Streatham", "state": "London", "name": "UK"}}}})
	case "/orders/item-categories":
		ok(w, map[string]any{"data": map[string]any{"categories": []any{map[string]any{"id": 1, "name": "Food"}}}})
	case "/orders/export-price":
		q := r.URL.Query()
		if q.Get("weightId") == "" && body["weightId"] == nil {
			m.t.Error("export-price: weightId not sent")
		}
		ok(w, map[string]any{"data": map[string]any{"price": "17500.00", "discountedRate": "17500.00", "discount": "N", "orderLocked": "N"}})
	case "/orders/import-price":
		if body["destinationState"] == nil || body["importLocationId"] == nil || body["weight"] == nil {
			m.t.Errorf("import-price: bad body %v", body)
		}
		ok(w, map[string]any{"data": map[string]any{"price": "20.00", "currency": "£"}})
	}
}

func TestMockOrderLifecycle(t *testing.T) {
	m, c := newMock(t)
	ctx := context.Background()
	if _, err := c.Authenticate(ctx, "me@x.com", "pw"); err != nil {
		t.Fatal(err)
	}
	cr, err := c.CreateOrders(ctx, []CreateOrder{{RecipientAddress: "a", RecipientState: "Lagos", RecipientName: "Femi", RecipientPhone: "0800",
		UniqueID: "u1", BatchID: "b", ValueOfItem: "5000", Weight: 1, WaybillNumber: "wb1", Fragile: true}})
	if err != nil {
		t.Fatal(err)
	}
	no := cr.OrderNos["u1"]
	if no == "" {
		t.Fatal("no order number")
	}
	if g, err := c.GetOrder(ctx, no); err != nil || g.OrderDetails[0].OrderStatus != "Pending Pick-Up" || g.OrderDetails[0].Cost != "3398.36" {
		t.Fatalf("get: %+v %v", g, err)
	}
	if tr, err := c.TrackOrder(ctx, no); err != nil || len(tr.History) != 1 {
		t.Fatalf("track: %+v %v", tr, err)
	}
	if u, err := c.ManifestURL(ctx, no); err != nil || u != "https://files/"+no+".pdf" {
		t.Fatalf("manifest %q %v", u, err)
	}
	if w, err := c.SearchByWaybill(ctx, "wb1"); err != nil || w.Data.OrderNumber != no {
		t.Fatalf("waybill %+v %v", w, err)
	}
	if _, err := c.UpdateOrders(ctx, []UpdateOrder{{OrderNo: no, RecipientName: "New"}}); err != nil || m.orders[no]["recipientName"] != "New" {
		t.Fatalf("update: %v %v", err, m.orders[no])
	}
	if s, err := c.SearchOrders(ctx, SearchOrders{StartDate: "2026-01-01", EndDate: "2026-12-31", OrderNo: no}); err != nil || s.Orders.Total != 1 || s.Orders.Data[0].RecipientName != "New" {
		t.Fatalf("search %+v %v", s, err)
	}
	if st, err := c.OrderStats(ctx, "2026-01-01", ""); err != nil || len(st.OrderDetails) != 1 {
		t.Fatalf("stats %v", err)
	}
	if err := c.CancelOrder(ctx, no, "test"); err != nil || m.status[no] != "Cancelled" {
		t.Fatalf("cancel %v", err)
	}
	err = c.CancelOrder(ctx, no, "again")
	if ae, ok := err.(*APIError); !ok || ae.HTTPStatus != 400 {
		t.Fatalf("second cancel should be 400, got %v", err)
	}
	if ae, _ := c.CancelOrder(ctx, "NOPE", "x"), 0; ae == nil {
		t.Fatal("cancel unknown should fail")
	}
	cr2, _ := c.CreateOrders(ctx, []CreateOrder{{RecipientAddress: "a", RecipientState: "Lagos", RecipientName: "n", RecipientPhone: "1", UniqueID: "u2", BatchID: "b", ValueOfItem: "1", Weight: 1}})
	if err := c.DeleteOrder(ctx, cr2.OrderNos["u2"]); err != nil || len(m.orders) != 1 {
		t.Fatalf("delete %v", err)
	}
}

func TestMockInsufficientWallet(t *testing.T) {
	m, c := newMock(t)
	c.SetCredentials("TOK", "SK")
	m.wallet = false
	r, err := c.CreateOrders(context.Background(), []CreateOrder{{UniqueID: "u1"}})
	ae, ok := err.(*APIError)
	if !ok || ae.HTTPStatus != 400 || r.OrderNos["u1"] != "Insufficient wallet balance." {
		t.Fatalf("got %+v %v", r, err)
	}
}

func TestMockInternational(t *testing.T) {
	_, c := newMock(t)
	ctx := context.Background()
	c.SetCredentials("TOK", "SK")
	el, err := c.ExportLocations(ctx)
	if err != nil || el.Data.ExportLocations[0].Name != "Canada" || el.Data.ExportWeights[0].ID != 1 {
		t.Fatal(err)
	}
	il, err := c.ImportLocations(ctx)
	if err != nil || len(il) != 2 || il[0].City != nil || *il[1].City != "Streatham" {
		t.Fatal(il, err)
	}
	cats, err := c.ImportItemCategories(ctx)
	if err != nil || cats[0].Name != "Food" {
		t.Fatal(err)
	}
	if xc, err := c.ExportDeliveryCost(ctx, ExportCostRequest{WeightID: 1, ExportLocationID: 1}); err != nil || xc.Data.Price != "17500.00" {
		t.Fatal(xc, err)
	}
	if ic, err := c.ImportDeliveryCost(ctx, ImportCostRequest{DestinationState: "Kano", ImportLocationID: 2, Weight: 5}); err != nil || ic.Data.Currency != "£" {
		t.Fatal(ic, err)
	}
	ex, err := c.CreateExportOrders(ctx, []ExportOrder{{RecipientAddress: "a", RecipientName: "n", RecipientPhone: "1", UniqueID: "x1", BatchID: "b", ValueOfItem: "1", Weight: 1, ExportLocationID: 1, Sender: Sender{ThirdParty: "true", SenderName: "S"}}})
	if err != nil || ex.OrderNos["x1"] == "" {
		t.Fatal(err)
	}
	im, err := c.CreateImportOrders(ctx, []ImportOrder{{RecipientAddress: "a", RecipientState: "Lagos", RecipientName: "n", RecipientPhone: "1", UniqueID: "i1", BatchID: "b", ValueOfItem: "1", Weight: 1, ImportLocationID: 2, ItemCategory: 1, BusinessName: "V", Quantity: 5}})
	if err != nil || im.OrderNos["i1"] == "" {
		t.Fatal(err)
	}
}

func TestMockMiscAndAuth(t *testing.T) {
	_, c := newMock(t)
	ctx := context.Background()
	if _, err := c.Authenticate(ctx, "me@x.com", "wrong"); err == nil {
		t.Fatal("bad login accepted")
	}
	c.Authenticate(ctx, "me@x.com", "pw")
	if c.TokenExpiry() == "" {
		t.Error("expiry not stored")
	}
	cost, err := c.DeliveryCost(ctx, CostRequest{State: "Kano"})
	if err != nil || cost.TotalCost != "6756.25" || !cost.Surcharge.HasSurcharge {
		t.Fatal(cost, err)
	}
	if sc, err := cost.StateCost(); err != nil || sc.State != "Kano" || sc.Cost != "6181.25" {
		t.Fatal(sc, err)
	}
	if e, err := c.DeliveryTimeEstimate(ctx, DeliveryTimeEstimateRequest{DeliveryType: "local", PickUpState: "Edo"}); err != nil || e.Data.ETA == "" {
		t.Fatal(err)
	}
	if h, err := c.Hubs(ctx, 2); err != nil || h[0].Name != "Lagoswa" {
		t.Fatal(err)
	}
	if l, err := c.Lockers(ctx, "Lagos"); err != nil || l.MaxWeight != "5" || l.Lockers[0].LockerID != "2100015346" {
		t.Fatal(l, err)
	}
	if a, err := c.LockerAvailable(ctx, "2100015346"); err != nil || !a {
		t.Fatal(err)
	}
	id, err := c.CreateUser(ctx, CreateUser{Fullname: "N", Email: "n@x.com", Password: "p", Role: "basic"})
	if err != nil || id != "G-new" {
		t.Fatal(id, err)
	}
	us, err := c.Users(ctx)
	if err != nil || us[0].ID != 71 {
		t.Fatal(err)
	}
	if err := c.DeleteUser(ctx, "71"); err != nil {
		t.Fatal(err)
	}
	if err := c.ChangePassword(ctx, "", "old", "new"); err != nil {
		t.Fatal(err)
	}
	if err := c.Logout(ctx, ""); err != nil {
		t.Fatal(err)
	}
	// missing credentials => 401 with API description
	_, err = New(WithBaseURL(c.baseURL)).Users(ctx)
	if ae, ok := err.(*APIError); !ok || ae.HTTPStatus != 401 || ae.Description != "Organization Secret Key is Required" {
		t.Fatalf("got %v", err)
	}
}

func TestMockWebhookEndToEnd(t *testing.T) {
	m, c := newMock(t)
	ctx := context.Background()
	c.SetCredentials("TOK", "SK")

	got := make(chan WebhookEvent, 1)
	app := httptest.NewServer(WebhookHandler("SK", func(e WebhookEvent) { got <- e }))
	defer app.Close()

	r, err := c.RegisterWebhook(ctx, app.URL)
	if err != nil || r.Data.Webhook != app.URL || m.hook != app.URL || r.Data.Webhooks[0].IsActive != "1" {
		t.Fatal(r, err)
	}

	deliver := func(sig, ts string) int {
		b, _ := json.Marshal(WebhookEvent{OrderNumber: "FZ1", Status: "Delivered"})
		req, _ := http.NewRequest("POST", m.hook, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Timestamp", ts)
		req.Header.Set("X-Signature", sig)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	if code := deliver(Sign("SK", "FZ1", "Delivered", ts), ts); code != 200 {
		t.Fatalf("valid delivery got %d", code)
	}
	select {
	case e := <-got:
		if e.OrderNumber != "FZ1" || e.Status != "Delivered" {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("handler not called")
	}
	if code := deliver(Sign("WRONG", "FZ1", "Delivered", ts), ts); code != 401 {
		t.Errorf("forged signature got %d", code)
	}
	old := strconv.FormatInt(time.Now().Add(-time.Hour).Unix(), 10)
	if code := deliver(Sign("SK", "FZ1", "Delivered", old), old); code != 401 {
		t.Errorf("replayed old timestamp got %d", code)
	}
	if code := deliver("", ts); code != 401 {
		t.Errorf("missing signature got %d", code)
	}
}
