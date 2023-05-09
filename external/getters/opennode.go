package getters

import (
	"fmt"
	"net/http"
	"encoding/json"
	"io/ioutil"
	"bytes"

	"github.com/kodylow/base58-website/internal/types"
	"github.com/kodylow/base58-website/internal/config"
)

const CHARGES_ENDPOINT string = "/charges"

func InitOpenNodeCheckout(ctx *config.AppContext, on *types.OpenNodeConfig, c *types.Checkout) (*types.OpenNodePayment, error) {

	amt := c.ComputeTotal(c.Type)
	callback := ctx.CallbackPath() + "/opennode-hook"
	success := fmt.Sprintf("%s/success?s=%s", ctx.CallbackPath(), c.SessionID)

	onReq := &types.OpenNodeRequest{
		Amount: float64(amt),
		Description: c.MakeDesc(),
		Currency: "USD",
		CustomerEmail: c.Email,
		NotifEmail: c.Email,
		OrderID: c.RegisterID,
		CallbackURL: callback,
		SuccessURL: success,
		AutoSettle: false,
		TTL: 360,
	}

	payload, err := json.Marshal(onReq)
	if err != nil {
		return nil, err
	}

	chargesURL := on.Endpoint + CHARGES_ENDPOINT
	req, err := http.NewRequest("POST", chargesURL, bytes.NewBuffer(payload))
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
		return nil, fmt.Errorf("error returned from opennode %s", err.Error())
	}

	var onresp types.OpenNodeResponse
	json.Unmarshal(body, &onresp)

	return onresp.Data, nil
}
