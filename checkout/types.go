// Package checkout retains data types for decoding pre-retirement sessions.
// It contains no payment, pricing, registration, or cart-writing behavior.
package checkout

import "encoding/gob"

type (
	Currency  string
	ShirtSize string

	Price struct {
		Unit  Currency `json:"currency"`
		Value uint64   `json:"amount"`
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
		// Historical fields retained for gob decoding only.
		LookupID  string        `json:"lookup"`
		Items     []any         `json:"items"`
		Token     string        `json:"token"`
		Discounts []string      `json:"discounts"`
		Infos     *CheckoutData `json:"infos"`
		BtcOk     bool          `json:"btc_ok"`
		FiatOk    bool          `json:"fiat_ok"`
	}

	CheckoutData struct {
		Email       string     `json:"email"`
		ShirtSize   *ShirtSize `json:"shirt_size,omitempty"`
		MailingAddr *string    `json:"mail_addr,omitempty"`
	}
)

func RegisterCartSerialization() {
	gob.Register(Price{})
	gob.Register(Item{})
	gob.Register(Cart{})
	gob.Register(CheckoutData{})
}
