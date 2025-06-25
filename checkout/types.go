package checkout

import (
	"bytes"
	"encoding/base64"
	"encoding/gob"
	"fmt"
	"strings"
)

/* Texas Sales Tax Rate */
const TAX_RATE float64 = float64(0.0825)

type (
	Currency    string
	ShirtSize   string
	CheckoutOpt string

	Price struct {
		Unit  Currency `json:"currency"`
		Value uint64   `json:'amount"`
	}

	CartItem interface {
		/* Context for doing calls etc, cart, paymentID */
		OnCallback(interface{}, *Cart, string) error
		GetID() string
		GetLookupID() string
		GetDisplayName() string
		GetDetails() string
		GetOptions() string
		GetQty() uint
		GetPrice() Price
		GetImgURL() string
		IsTaxed() bool
	}

	Item struct {
		ID          string `json:"item_id"`
		LookupID    string `json:"lookup"`
		DisplayName string `json:"name"`
		Details     string `json:"details"`
		Options     string `json:"options"`
		Qty         uint   `json:"qty"`
		Price       Price  `json:"price"`
		ImgURL      string `json:"img_url"`
		Taxed       bool   `json:"is_taxed"`
	}

	Cart struct {
		/* We save carts before heading to checkout */
		LookupID  string        `json:"lookup"`
		Items     []CartItem    `json:"items"`
		Token     string        `json:"token"`
		Discounts []string      `json:"discounts"`
		Infos     *CheckoutData `json:"infos"`
		BtcOk     bool          `json:'btc_ok"`
		FiatOk    bool          `json:"fiat_ok"`
	}

	CheckoutData struct {
		Email       string     `json:"email"`
		ShirtSize   *ShirtSize `json:"shirt_size,omitempty"`
		MailingAddr *string    `json:"mail_addr,omitempty"`
	}

	SummaryPage struct {
		CheckoutVia CheckoutOpt
	}
)

const (
	Bitcoin CheckoutOpt = "btc"
	Fiat    CheckoutOpt = "fiat"
)

func (item Item) GetID() string {
	return item.ID
}

func (item Item) GetLookupID() string {
	return item.LookupID
}

func (item Item) GetDisplayName() string {
	return item.DisplayName
}

func (item Item) GetDetails() string {
	return item.Details
}

func (item Item) GetOptions() string {
	return item.Options
}

func (item Item) GetQty() uint {
	return item.Qty
}

func (item Item) GetPrice() Price {
	return item.Price
}

func (item Item) GetImgURL() string {
	return item.ImgURL
}

func (item Item) IsTaxed() bool {
	return item.Taxed
}

func ConvertCurrency(value uint64, from, to Currency) uint64 {
	/* FIXME: implement this! */
	return value
}

func (p Price) ValAs(currency Currency) uint64 {
	if p.Unit == currency {
		return p.Value
	}

	return ConvertCurrency(p.Value, p.Unit, currency)
}

/* Add up all the prices ! */
func (c *Cart) SubTotal(currency Currency) Price {
	total := uint64(0)

	for _, item := range c.Items {
		unitPrice := item.GetPrice().ValAs(currency)
		total += unitPrice * uint64(item.GetQty())
	}

	return Price{
		Value: total,
		Unit:  currency,
	}
}

/* FIXME: discount language/parser */
func (c *Cart) Discount(currency Currency) Price {
	discount := uint64(0)
	/* Caluclate the discount! */
	for _, code := range c.Discounts {
		if code == "6c-1" {
			for _, item := range c.Items {
				discount += uint64(item.GetQty()/6) * item.GetPrice().ValAs(currency)
			}
		}
	}

	return Price{
		Value: discount,
		Unit:  currency,
	}
}

func (c *Cart) Shipping(currency Currency) Price {
	return Price{
		Value: 0,
		Unit:  currency,
	}
}

/* FIXME: more dynamic pricing for taxes?! */
func (c *Cart) Taxes(currency Currency) Price {
	taxes := uint64(0)

	for _, item := range c.Items {
		if item.IsTaxed() {
			itemsCost := uint64(item.GetQty()) * item.GetPrice().ValAs(currency)
			taxes += uint64(float64(itemsCost) * TAX_RATE)
		}
	}

	return Price{
		Value: taxes,
		Unit:  currency,
	}
}

/* FIXME: add in tax calculations! */
func (c *Cart) Total(currency Currency) Price {
	subTotal := c.SubTotal(currency)
	discount := c.Discount(currency)
	taxes := c.Taxes(currency)
	shipping := c.Shipping(currency)

	if subTotal.Value < discount.Value {
		discount.Value = subTotal.Value
	}

	return Price{
		Value: subTotal.Value - discount.Value + taxes.Value + shipping.Value,
		Unit:  currency,
	}
}

func (p *Price) Display() string {
	switch p.Unit {
	case USD:
		dollars := p.Value / uint64(100)
		cents := p.Value % 100
		return fmt.Sprintf("$%0d.%02d", dollars, cents)
	case SATS:
		return fmt.Sprintf("%dsats", p.Value)
	default:
		return fmt.Sprintf("%d", p.Value)
	}
}

func RegisterCartSerialization() {
	gob.Register(Price{})
	gob.Register(Item{})
	gob.Register(Cart{})
	gob.Register(CheckoutData{})
}

const (
	USD  Currency = "usd"
	SATS Currency = "sats"
)

const (
	Small ShirtSize = "small"
	Med   ShirtSize = "med"
	Large ShirtSize = "large"
	XL    ShirtSize = "xl"
	XXL   ShirtSize = "xxl"
	XXXL  ShirtSize = "3xl"
)

func (s ShirtSize) String() string {
	return string(s)
}

var mapEnumShirtSize = func() map[string]ShirtSize {
	m := make(map[string]ShirtSize)
	m[string(Small)] = Small
	m[string(Med)] = Med
	m[string(Large)] = Large
	m[string(XL)] = XL
	m[string(XXL)] = XXL
	m[string(XXXL)] = XXXL

	return m
}()

func ParseShirtSize(str string) (ShirtSize, bool) {
	ss, ok := mapEnumShirtSize[strings.ToLower(str)]
	return ss, ok
}

func (curr Currency) String() string {
	return string(curr)
}

var mapEnumCurrency = func() map[string]Currency {
	m := make(map[string]Currency)
	m[string(USD)] = USD
	m[string(SATS)] = SATS

	return m
}()

func ParseCurrency(option string) (Currency, bool) {
	curr, ok := mapEnumCurrency[strings.ToLower(option)]
	return curr, ok
}

/* Produces a human readable description of all items in a cart */
func (c *Cart) MakeDesc() string {
	var desc strings.Builder

	for _, item := range c.Items {
		if desc.Len() > 0 {
			desc.WriteString(", ")
		}
		qtyStr := ""
		if item.GetQty() > 1 {
			qtyStr = "s"
		}
		fmt.Fprintf(&desc, "%d seat%s in Base58's %s class. %s", item.GetQty(), qtyStr, item.GetDisplayName(), item.GetOptions())
	}

	return desc.String()
}

func (cart *Cart) ToBase64() (string, error) {
	buf := new(bytes.Buffer)
	err := gob.NewEncoder(buf).Encode(cart)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func CartFromBase64(cartBin string) (*Cart, error) {
	cartBytes, err := base64.StdEncoding.DecodeString(cartBin)
	if err != nil {
		fmt.Printf("oh no! %s", err)
		return nil, err
	}

	var cart Cart
	buf := bytes.NewBuffer(cartBytes)
	err = gob.NewDecoder(buf).Decode(&cart)

	return &cart, err
}
