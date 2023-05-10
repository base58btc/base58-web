package types

import (
	"strings"
)

type (
	StripeConfig struct {
		Key         string
		Pubkey      string
		EndpointSec string
	}
)

func (s *StripeConfig) IsTest() bool {
	return strings.Contains(s.Key, "test")
}
