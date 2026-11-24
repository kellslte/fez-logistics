package fez

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"
)

// Mutating sandbox test. Runs only with FEZ_WRITE_TESTS=1. Creates orders and a
// user, then cancels/deletes them.
func TestSandboxWrite(t *testing.T) {
	loadEnv()
	if os.Getenv("FEZ_WRITE_TESTS") != "1" || os.Getenv("FEZ_USER_ID") == "" {
		t.Skip("set FEZ_WRITE_TESTS=1 and credentials to run")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	c := New()
	if _, err := c.Authenticate(ctx, os.Getenv("FEZ_USER_ID"), os.Getenv("FEZ_PASSWORD")); err != nil {
		t.Fatal(err)
	}
	step := func(name string, err error) bool {
		if err != nil {
			t.Errorf("%s: %v", name, err)
			if ae, ok := err.(*APIError); ok {
				t.Logf("body: %s", ae.Body)
			}
			return false
		}
		t.Logf("%s: ok", name)
		return true
	}
	run := strconv.FormatInt(time.Now().Unix(), 10)

	func() {
		// --- local order lifecycle ---
		uid, waybill := "gotest-"+run, "wb-"+run
		cr, err := c.CreateOrders(ctx, []CreateOrder{{
			RecipientAddress: "12 Test Street, Ikeja", RecipientState: "Lagos", RecipientName: "Go Test",
			RecipientPhone: "08000000000", RecipientEmail: "gotest@example.com", UniqueID: uid, BatchID: "gotest-b-" + run,
			ItemDescription: "sdk test", ValueOfItem: "5000", Weight: 1, PickUpState: "Lagos", WaybillNumber: waybill,
		}})
		var orderNo string
		if step("CreateOrders", err) {
			orderNo = cr.OrderNos[uid]
			t.Logf("orderNos=%v", cr.OrderNos)
			if orderNo == "" {
				t.Error("no order number returned")
				return
			}
		} else {
			return
		}
		cancelled := false
		defer func() {
			if !cancelled {
				_ = c.CancelOrder(context.Background(), orderNo, "sdk test cleanup")
			}
		}()

		if g, err := c.GetOrder(ctx, orderNo); step("GetOrder", err) {
			t.Logf("status=%s cost=%s", g.OrderDetails[0].OrderStatus, g.OrderDetails[0].Cost)
		}
		if tr, err := c.TrackOrder(ctx, orderNo); step("TrackOrder", err) {
			t.Logf("track status=%s history=%d", tr.Order.OrderStatus, len(tr.History))
		}
		if w, err := c.SearchByWaybill(ctx, waybill); step("SearchByWaybill", err) && w.Data.OrderNumber != orderNo {
			t.Errorf("waybill maps to %q want %q", w.Data.OrderNumber, orderNo)
		}
		_, err = c.UpdateOrders(ctx, []UpdateOrder{{OrderNo: orderNo, RecipientName: "Go Test Updated", RecipientPhone: "08011111111"}})
		step("UpdateOrders", err)
		if g, err := c.GetOrder(ctx, orderNo); err == nil && g.OrderDetails[0].RecipientName != "Go Test Updated" {
			t.Errorf("update not reflected: %q", g.OrderDetails[0].RecipientName)
		}
		if u, err := c.ManifestURL(ctx, orderNo); err != nil {
			t.Logf("ManifestURL (may legitimately 404 until generated): %v", err)
		} else {
			t.Logf("ManifestURL: %s", u)
		}
		if s, err := c.SearchOrders(ctx, SearchOrders{StartDate: time.Now().AddDate(0, 0, -1).Format("2006-01-02"), EndDate: time.Now().AddDate(0, 0, 1).Format("2006-01-02"), OrderNo: orderNo}); step("SearchOrders(orderNo)", err) {
			t.Logf("found=%d", s.Orders.Total)
		}
		step("CancelOrder", c.CancelOrder(ctx, orderNo, "sdk integration test"))
		cancelled = true

		// --- DeleteOrder on a second order ---
		uid2 := uid + "-del"
		if cr2, err := c.CreateOrders(ctx, []CreateOrder{{RecipientAddress: "1 Del St", RecipientState: "Lagos", RecipientName: "Del", RecipientPhone: "08000000000", UniqueID: uid2, BatchID: "gotest-b-" + run, ValueOfItem: "1000", Weight: 1}}); step("CreateOrders(2)", err) {
			step("DeleteOrder", c.DeleteOrder(ctx, cr2.OrderNos[uid2]))
		}

		// --- international ---
		if el, err := c.ExportLocations(ctx); err == nil && len(el.Data.ExportLocations) > 0 {
			id := fmt.Sprint(run, "-x")
			er, err := c.CreateExportOrders(ctx, []ExportOrder{{RecipientAddress: "1 Export Rd", RecipientName: "Exp", RecipientPhone: "08000000000",
				UniqueID: id, BatchID: "gotest-x-" + run, ValueOfItem: "20000", Weight: 1, ExportLocationID: el.Data.ExportLocations[0].ID, PickUpState: "Lagos"}})
			if step("CreateExportOrders", err) {
				step("CancelOrder(export)", c.CancelOrder(ctx, er.OrderNos[id], "sdk test"))
			}
		}
		if il, err := c.ImportLocations(ctx); err == nil {
			cats, _ := c.ImportItemCategories(ctx)
			if len(cats) > 0 {
				created := false
				for _, loc := range il {
					id := run + "-i" + strconv.Itoa(loc.ID)
					ir, err := c.CreateImportOrders(ctx, []ImportOrder{{RecipientAddress: "1 Import Rd", RecipientState: "Lagos", RecipientName: "Imp Test", RecipientPhone: "08000000000",
						UniqueID: id, BatchID: "gotest-i-" + run, ValueOfItem: "20000", Weight: 1, ImportLocationID: loc.ID, ItemCategory: cats[0].ID,
						BusinessName: "Test Vendor", OrderID: "o-" + run, TrackingID: "t-" + run, Quantity: 1}})
					if err != nil {
						t.Logf("CreateImportOrders loc %d (%s): %v", loc.ID, loc.Name, err)
						continue
					}
					created = true
					t.Logf("CreateImportOrders: ok via location %d (%s)", loc.ID, loc.Name)
					step("CancelOrder(import)", c.CancelOrder(ctx, ir.OrderNos[id], "sdk test"))
					break
				}
				if !created {
					t.Error("no import location accepted an order")
				}
			}
		}
	}()

	// --- lockers ---
	if l, err := c.Lockers(ctx, "Lagos"); step("Lockers", err) && len(l.Lockers) > 0 {
		av, err := c.LockerAvailable(ctx, l.Lockers[0].LockerID)
		step("LockerAvailable", err)
		t.Logf("locker %s available=%v", l.Lockers[0].LockerID, av)
	}

	// --- users ---
	email := "gotest+" + run + "@example.com"
	if id, err := c.CreateUser(ctx, CreateUser{Fullname: "Go Test", Email: email, Password: "Tmp#" + run + "aZ", Role: "basic"}); step("CreateUser", err) {
		t.Logf("user_id=%s", id)
		users, err := c.Users(ctx)
		step("Users", err)
		for _, u := range users {
			if u.Email == email {
				step("DeleteUser", c.DeleteUser(ctx, strconv.Itoa(u.ID)))
			}
		}
	}
}
