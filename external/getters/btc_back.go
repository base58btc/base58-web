package getters

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	lnsocket "github.com/jb55/lnsocket/go"
	"github.com/kodylow/base58-website/internal/types"
)

type LNError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type LNCmdoResponse struct {
	Error  json.RawMessage `json:"error"`
	Result json.RawMessage `json:"result"`
}

type Invoice struct {
	PaymentHash   string
	ExpiresAt     uint64
	Bolt11        string
	PaymentSecret string
}

func NewInvoice(cmdo *types.CommandoConfig, description string, amt uint64) (string, error) {
	/* Now we do the fetch the invoice from the LN node */
	ln := lnsocket.LNSocket{}
	ln.GenKey()

	err := ln.ConnectAndInit(cmdo.Host, cmdo.NodeID)
	if err != nil {
		return "", err
	}

	defer ln.Disconnect()

	/* Add random label */
	label := "b58web-" + strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	params := fmt.Sprintf("[\"%dmsat\", \"%s\", \"%s\"]", amt, label, description)
	body, err := ln.Rpc(cmdo.Rune, "invoice", params)
	if err != nil {
		return "", err
	}

	// Parse as error or result?
	var resp LNCmdoResponse
	err = json.NewDecoder(strings.NewReader(body)).Decode(&resp)
	if err != nil {
		return "", err
	}

	if resp.Error != nil {
		var errMsg LNError
		err = json.Unmarshal(resp.Error, &errMsg)
		if err != nil {
			return "", fmt.Errorf("%s", resp.Error)
		}

		return "", fmt.Errorf("%s", errMsg.Message)
	}

	if resp.Result == nil {
		return "", fmt.Errorf("No result for invoice call")
	}

	var inv Invoice
	err = json.Unmarshal(resp.Result, &inv)
	if err != nil {
		return "", err
	}

	return inv.Bolt11, nil
}
