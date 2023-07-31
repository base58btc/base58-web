package handlers

import (
	"testing"
)

var testWIF = []struct {
	val       string
	expectWIF string
}{
	{"888", "cMahea7zqjxrtgAbB7LSGbcQUr1uX1ojuat9jZodMN8EvKBjKEAz"},
}

func TestWIF(t *testing.T) {
	for _, test := range testWIF {
		resultWIF, err := MakeWIF(test.val, DEFAULT_NETWORK_BYTE)
		if err != nil {
			t.Error(err)
			continue
		}

		if resultWIF != test.expectWIF {
			t.Errorf("WIF test failed. Expected %s, got %s", test.expectWIF, resultWIF)
			continue
		}
	}
}
