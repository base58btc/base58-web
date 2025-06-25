package getters

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/kodylow/base58-website/internal/config"
	"github.com/kodylow/base58-website/internal/types"
)

const CHARGES_ENDPOINT string = "/charges"

type OpenCheckoutInfo struct {
	Amount      float64
	Description string
	Email       string
	OrderID     string
	SuccessID   string
}

func InitOpenNodeCheckout(ctx *config.AppContext, on *types.OpenNodeConfig, c OpenCheckoutInfo) (*types.OpenNodePayment, error) {

	callback := ctx.CallbackPath() + "/opennode-hook"
	success := fmt.Sprintf("%s/success?s=%s", ctx.CallbackPath(), c.SuccessID)

	onReq := &types.OpenNodeRequest{
		Amount:        c.Amount,
		Description:   c.Description,
		Currency:      "USD",
		CustomerEmail: c.Email,
		NotifEmail:    c.Email,
		OrderID:       c.OrderID,
		CallbackURL:   callback,
		SuccessURL:    success,
		AutoSettle:    false,
		TTL:           360,
	}

	payload, err := json.Marshal(onReq)
	if err != nil {
		return nil, err
	}

	chargesURL := on.Endpoint + CHARGES_ENDPOINT
	req, _ := http.NewRequest("POST", chargesURL, bytes.NewBuffer(payload))
	req.Header.Add("Authorization", on.Key)
	req.Header.Add("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("error returned from opennode %d: %s", resp.StatusCode, body)
	}

	var onresp types.OpenNodeResponse
	json.Unmarshal(body, &onresp)

	return onresp.Data, nil
}
