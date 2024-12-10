package checkout

import (
	"testing"
  "encoding/gob"

  "github.com/google/go-cmp/cmp"
)

type TestItem struct {
    Item
}

func (ti TestItem) OnCallback(appCtx interface{}, cart *Cart, paymentID string) error {
  return nil
}

func TestCartSerialization(t *testing.T) {

  RegisterCartSerialization()
  gob.Register(TestItem{})

  shirtSize := Small
  addr :=  "101 Dalmation Way, Brazola County"

  cart := Cart{
    LookupID: "lookupid1",
    Items: []CartItem{
      TestItem{ Item{
        ID: "id1",
        DisplayName: "Ginger Cookies",
        Details: "delicious to eat",
        Options: "comes in ginger color",
        Qty: uint(2),
        Price: Price{
          Unit: USD,
          Value: uint64(1000),
        },
        ImgURL: "https://moon.baby",
        Taxed: false,
      }},
      TestItem{ Item{
        ID: "id2",
        DisplayName: "Saba Balls",
        Details: "Super high bounce balls",
        Options: "green and purple",
        Qty: uint(5),
        Price: Price{
          Unit: SATS,
          Value: uint64(21000),
        },
        ImgURL: "https://moon.baby",
        Taxed: false,
      }},
    },
    Token: "onlyuseonce",
    Discounts: []string{"disco54", "hellomoto"},
    Infos: &CheckoutData{
      Email: "xyz@hex.lol",
      ShirtSize:  &shirtSize,
      MailingAddr: &addr,
    },
    BtcOk: true,
    FiatOk: true,
  }

  serial, err := cart.ToBase64()
  if err != nil {
    t.Fatalf("serialization failed %v", err)
  }

  cart2, err := CartFromBase64(serial)
  if err != nil {
    t.Fatalf("de-serialization failed %v", err)
  }

  if diff := cmp.Diff(&cart, cart2); diff != "" {
    t.Errorf("Carts not equal. %s", diff)
  }
}
