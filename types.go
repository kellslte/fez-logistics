package fez

import "encoding/json"

// Number decodes JSON numbers or numeric strings (the API mixes both).
type Number = json.Number

// ---- Auth ----

type AuthResponse struct {
	Envelope
	AuthDetails struct {
		AuthToken   string `json:"authToken"`
		ExpireToken string `json:"expireToken"`
	} `json:"authDetails"`
	UserDetails struct {
		UserID   string `json:"userID"`
		FullName string `json:"Full Name"`
		Username string `json:"Username"`
	} `json:"userDetails"`
	OrgDetails struct {
		SecretKey   string `json:"secret-key"`
		OrgFullName string `json:"Org Full Name"`
	} `json:"orgDetails"`
}

// ---- Orders ----

// ThirdParty fields let you override the sender (set ThirdParty to "true").
type Sender struct {
	ThirdParty    string `json:"thirdparty,omitempty"`
	SenderName    string `json:"senderName,omitempty"`
	SenderAddress string `json:"senderAddress,omitempty"`
	SenderPhone   string `json:"senderPhone,omitempty"`
}

// CreateOrder is one local delivery request (POST /order takes an array).
type CreateOrder struct {
	RecipientAddress        string  `json:"recipientAddress"`
	RecipientState          string  `json:"recipientState"`
	RecipientName           string  `json:"recipientName"`
	RecipientPhone          string  `json:"recipientPhone"`
	RecipientEmail          string  `json:"recipientEmail,omitempty"`
	RecipientAlternatePhone string  `json:"recipientAlternatePhone,omitempty"`
	UniqueID                string  `json:"uniqueID"`
	BatchID                 string  `json:"BatchID"`
	CustToken               string  `json:"CustToken,omitempty"`
	ItemDescription         string  `json:"itemDescription,omitempty"`
	AdditionalDetails       string  `json:"additionalDetails,omitempty"`
	ValueOfItem             string  `json:"valueOfItem"`
	Weight                  int     `json:"weight"`
	PickUpState             string  `json:"pickUpState,omitempty"`
	PickUpAddress           string  `json:"pickUpAddress,omitempty"`
	PickUpDate              string  `json:"pickUpDate,omitempty"`
	WaybillNumber           string  `json:"waybillNumber,omitempty"`
	IsItemCod               bool    `json:"isItemCod,omitempty"`
	CashOnDeliveryAmount    float64 `json:"cashOnDeliveryAmount,omitempty"`
	Fragile                 bool    `json:"fragile,omitempty"`
	LockerID                string  `json:"lockerID,omitempty"`
	Sender
}

// CreateOrdersResponse maps your uniqueID to the Fez order number.
type CreateOrdersResponse struct {
	Envelope
	OrderNos map[string]string `json:"orderNos"`
}

// UpdateOrder updates recipient details of an existing order.
type UpdateOrder struct {
	OrderNo          string `json:"orderNo"`
	RecipientAddress string `json:"recipientAddress,omitempty"`
	RecipientState   string `json:"recipientState,omitempty"`
	RecipientName    string `json:"recipientName,omitempty"`
	RecipientPhone   string `json:"recipientPhone,omitempty"`
}

type UpdateOrdersResponse struct {
	Envelope
	Response map[string]string `json:"Response"`
}

// OrderDetail is the (large) order object returned by GetOrder.
type OrderDetail struct {
	ID                      int64           `json:"id"`
	OrderNo                 string          `json:"orderNo"`
	ClientAddress           string          `json:"clientAddress"`
	DeliveryOTP             any             `json:"delivery_OTP"`
	PickUpDate              *string         `json:"pickUpDate"`
	Quantity                int             `json:"quantity"`
	DispatchDate            *string         `json:"dispatchDate"`
	RecipientName           string          `json:"recipientName"`
	RecipientAddress        string          `json:"recipientAddress"`
	RecipientEmail          *string         `json:"recipientEmail"`
	RecipientPhone          string          `json:"recipientPhone"`
	RecipientAlternatePhone *string         `json:"recipientAlternatePhone"`
	RecipientState          string          `json:"recipientState"`
	SenderState             *string         `json:"senderState"`
	ItemDescription         string          `json:"itemDescription"`
	StatusDescription       *string         `json:"statusDescription"`
	OrderStatus             string          `json:"orderStatus"`
	DeliveryDate            *string         `json:"deliveryDate"`
	PaymentMode             *string         `json:"paymentMode"`
	PaymentStatus           string          `json:"paymentStatus"`
	Cost                    Number          `json:"cost"`
	DropZoneName            *string         `json:"dropZoneName"`
	ReturnReason            *string         `json:"returnReason"`
	ReturnDate              *string         `json:"returnDate"`
	ReceivedBySignature     *string         `json:"recievedBySignature"`
	AdditionalNote          *string         `json:"additionalNote"`
	OrderVerified           string          `json:"orderVerified"`
	CreatedAt               string          `json:"created_at"`
	CreatedBy               string          `json:"createdBy"`
	OrgRep                  string          `json:"OrgRep"`
	OrderDate               string          `json:"orderDate"`
	Weight                  Number          `json:"weight"`
	Manifest                json.RawMessage `json:"manifest"`
	Client                  json.RawMessage `json:"client"`
	OrganizationUser        json.RawMessage `json:"organizationUser"`
	Currency                string          `json:"currency"`
	CurrencySymbol          string          `json:"currencySymbol"`
	ManifestPrintOutURL     *string         `json:"manifest_print_out_url"`
	IsItemCod               Number          `json:"is_item_cod"`
	CodAmount               Number          `json:"cod_amount"`
	Insured                 bool            `json:"insured"`
	InsuranceStatus         string          `json:"insurance_status"`
	Timezone                string          `json:"timezone"`
}

type GetOrderResponse struct {
	Envelope
	OrderDetails []OrderDetail `json:"orderDetails"`
}

type ManifestResponse struct {
	Envelope
	Data struct {
		URL string `json:"url"`
	} `json:"data"`
}

// SearchOrders filters POST /orders/search. StartDate, EndDate and Page are required.
type SearchOrders struct {
	StartDate      string `json:"startDate"`
	EndDate        string `json:"endDate"`
	Page           int    `json:"page"`
	OrderNo        string `json:"orderNo,omitempty"`
	RecipientName  string `json:"recipientName,omitempty"`
	RecipientPhone string `json:"recipientPhone,omitempty"`
	OrderStatus    string `json:"orderStatus,omitempty"`
	OrgRep         string `json:"OrgRep,omitempty"`
}

// OrderSummary is a row in search / stats results.
type OrderSummary struct {
	OrderNo           string  `json:"orderNo"`
	RecipientName     string  `json:"recipientName"`
	RecipientEmail    string  `json:"recipientEmail"`
	RecipientAddress  string  `json:"recipientAddress"`
	RecipientPhone    string  `json:"recipientPhone"`
	OrderStatus       string  `json:"orderStatus"`
	StatusDescription *string `json:"statusDescription"`
	Cost              Number  `json:"cost"`
	CreatedBy         string  `json:"createdBy"`
	OrgRep            string  `json:"OrgRep"`
	OrderDate         string  `json:"orderDate"`
	PickUpDate        *string `json:"pickUpDate"`
	DispatchDate      *string `json:"dispatchDate"`
	DeliveryDate      *string `json:"deliveryDate"`
	ReturnReason      *string `json:"returnReason"`
	ReturnDate        *string `json:"returnDate"`
	DropZoneName      *string `json:"dropZoneName"`
}

type SearchOrdersResponse struct {
	Envelope
	Orders struct {
		CurrentPage  int            `json:"current_page"`
		Data         []OrderSummary `json:"data"`
		FirstPageURL string         `json:"first_page_url"`
		From         int            `json:"from"`
		LastPage     int            `json:"last_page"`
		LastPageURL  string         `json:"last_page_url"`
		NextPageURL  *string        `json:"next_page_url"`
		Path         string         `json:"path"`
		PerPage      int            `json:"per_page"`
		PrevPageURL  *string        `json:"prev_page_url"`
		To           int            `json:"to"`
		Total        int            `json:"total"`
	} `json:"orders"`
}

type WaybillSearchResponse struct {
	Envelope
	Data struct {
		OrderNumber   string `json:"orderNumber"`
		WaybillNumber string `json:"waybillNumber"`
	} `json:"data"`
}

type StatsResponse struct {
	Envelope
	OrderDetails []OrderSummary `json:"orderDetails"`
}

type TrackResponse struct {
	Envelope
	Order struct {
		OrderNo          string  `json:"orderNo"`
		OrderStatus      string  `json:"orderStatus"`
		RecipientAddress string  `json:"recipientAddress"`
		RecipientName    string  `json:"recipientName"`
		RecipientState   string  `json:"recipientState"`
		SenderAddress    string  `json:"senderAddress"`
		SenderName       string  `json:"senderName"`
		CreatedAt        string  `json:"createdAt"`
		Timezone         string  `json:"timezone"`
		ProofOfDelivery  *string `json:"proofOfDelivery"`
	} `json:"order"`
	History []struct {
		OrderStatus        string `json:"orderStatus"`
		StatusCreationDate string `json:"statusCreationDate"`
		StatusDescription  string `json:"statusDescription"`
	} `json:"history"`
}

// CostRequest: all fields optional. Empty State returns costs for every state.
type CostRequest struct {
	State       string  `json:"state,omitempty"`
	PickUpState string  `json:"pickUpState,omitempty"`
	Weight      float64 `json:"weight,omitempty"`
	Locker      bool    `json:"locker,omitempty"`
}

type StateCost struct {
	State string `json:"state"`
	Cost  Number `json:"cost"`
}

// CostResponse. The docs return the cost under both "Cost" and "cost"; "cost"
// is the one to use. The amount charged is TotalCost (cost + VAT + surcharge).
// Cost may be an object (single state) or array (all states) so it is raw.
type CostResponse struct {
	Envelope
	Cost json.RawMessage `json:"cost"`
	VAT  struct {
		VatAmount  Number `json:"vatAmount"`
		VatPercent Number `json:"vatPercent"`
	} `json:"vat"`
	Surcharge struct {
		HasSurcharge bool `json:"hasSurcharge"`
		Items        []struct {
			ID              int    `json:"id"`
			Name            string `json:"name"`
			SurgeType       string `json:"surge_type"`
			Rate            Number `json:"rate"`
			CalculationType string `json:"calculation_type"`
			Amount          Number `json:"amount"`
		} `json:"items"`
		TotalAmount Number `json:"totalAmount"`
	} `json:"surcharge"`
	TotalCost Number `json:"totalCost"`
}

// StateCost decodes Cost when a single state was requested.
func (r *CostResponse) StateCost() (StateCost, error) {
	var s StateCost
	err := json.Unmarshal(r.Cost, &s)
	return s, err
}

// DeliveryTimeEstimateRequest.DeliveryType is "import", "export" or "local".
type DeliveryTimeEstimateRequest struct {
	DeliveryType string `json:"delivery_type"`
	PickUpState  string `json:"pick_up_state,omitempty"`
	DropOffState string `json:"drop_off_state,omitempty"`
}

type DeliveryTimeEstimateResponse struct {
	Envelope
	Data struct {
		ETA string `json:"eta"`
	} `json:"data"`
}

type State struct {
	ID    int    `json:"id"`
	State string `json:"state"`
}

type StatesResponse struct {
	Envelope
	States []State `json:"states"`
}

type Hub struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Address string `json:"address"`
}

type HubsResponse struct {
	Envelope
	Hubs []Hub `json:"hubs"`
}

// ---- International ----

type ExportOrder struct {
	RecipientAddress  string `json:"recipientAddress"`
	RecipientName     string `json:"recipientName"`
	RecipientPhone    string `json:"recipientPhone"`
	PickUpState       string `json:"pickUpState,omitempty"`
	UniqueID          string `json:"uniqueID"`
	BatchID           string `json:"BatchID"`
	CustToken         string `json:"CustToken,omitempty"`
	ItemDescription   string `json:"itemDescription,omitempty"`
	AdditionalDetails string `json:"additionalDetails,omitempty"`
	ValueOfItem       string `json:"valueOfItem"`
	Weight            int    `json:"weight"`
	ExportLocationID  int    `json:"exportLocationId"`
	Sender
}

type ImportOrder struct {
	RecipientAddress  string `json:"recipientAddress"`
	RecipientState    string `json:"recipientState"`
	RecipientName     string `json:"recipientName"`
	RecipientPhone    string `json:"recipientPhone"`
	RecipientEmail    string `json:"recipientEmail,omitempty"`
	UniqueID          string `json:"uniqueID"`
	BatchID           string `json:"BatchID"`
	CustToken         string `json:"CustToken,omitempty"`
	ItemDescription   string `json:"itemDescription,omitempty"`
	AdditionalDetails string `json:"additionalDetails,omitempty"`
	ValueOfItem       string `json:"valueOfItem"`
	Weight            int    `json:"weight"`
	ImportLocationID  int    `json:"importLocationId"`
	ItemCategory      int    `json:"itemCategory"`
	BusinessName      string `json:"businessName"`
	TrackingID        any    `json:"trackingId,omitempty"`
	OrderID           any    `json:"orderId,omitempty"`
	Quantity          any    `json:"quantity,omitempty"`
	Sender
}

type ExportCostRequest struct {
	WeightID         int    `json:"weightId"`
	ExportLocationID int    `json:"exportLocationId"`
	PickUpState      string `json:"pickUpState,omitempty"`
}

type ExportCostResponse struct {
	Envelope
	Data struct {
		Price          Number `json:"price"`
		DiscountedRate Number `json:"discountedRate"`
		Discount       string `json:"discount"`
		OrderLocked    string `json:"orderLocked"`
	} `json:"data"`
}

type ImportCostRequest struct {
	DestinationState string `json:"destinationState"`
	Weight           int    `json:"weight"`
	ImportLocationID int    `json:"importLocationId"`
}

type ImportCostResponse struct {
	Envelope
	Data struct {
		Price    Number `json:"price"`
		Currency string `json:"currency"`
	} `json:"data"`
}

type IDName struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type ExportLocationsResponse struct {
	Envelope
	Data struct {
		ExportLocations []IDName `json:"exportLocations"`
		ExportWeights   []IDName `json:"exportWeights"`
	} `json:"data"`
}

type ImportLocation struct {
	ID    int     `json:"id"`
	City  *string `json:"city"`
	State *string `json:"state"`
	Name  string  `json:"name"`
}

type ImportLocationsResponse struct {
	Envelope
	Data struct {
		ImportLocations []ImportLocation `json:"importLocations"`
	} `json:"data"`
}

type ItemCategoriesResponse struct {
	Envelope
	Data struct {
		Categories []IDName `json:"categories"`
	} `json:"data"`
}

// ---- Lockers ----

type Locker struct {
	LockerID      string `json:"lockerID"`
	LockerAddress string `json:"lockerAddress"`
}

type LockersResponse struct {
	Envelope
	Lockers        []Locker `json:"Lockers"`
	MaxWeight      Number   `json:"maxWeight"`
	MaxValueOfItem Number   `json:"maxValueOfItem"`
}

type LockerAvailabilityResponse struct {
	Envelope
	Available bool `json:"Available"`
}

// ---- Users ----

// CreateUser.Role is "basic" or "admin".
type CreateUser struct {
	Fullname string `json:"fullname"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

type CreateUserResponse struct {
	Envelope
	UserID string `json:"user_id"`
}

type User struct {
	ID        int    `json:"id"`
	UserID    string `json:"user_id"`
	Fullname  string `json:"fullname"`
	Email     string `json:"email"`
	CreatedAt string `json:"created_at"`
}

type UsersResponse struct {
	Envelope
	Users []User `json:"users"`
}

// ---- Webhooks ----

type WebhookEntry struct {
	URL      string `json:"url"`
	Type     string `json:"type"`
	IsActive Number `json:"is_active"`
}

type RegisterWebhookResponse struct {
	Envelope
	Data struct {
		Webhook  string         `json:"webhook"`
		Webhooks []WebhookEntry `json:"webhooks"`
	} `json:"data"`
}

// WebhookEvent is the JSON body Fez POSTs to your registered URL.
type WebhookEvent struct {
	OrderNumber string `json:"orderNumber"`
	Status      string `json:"status"`
}
