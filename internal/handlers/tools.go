package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/akamensky/base58"
	"github.com/btcsuite/btcutil/bech32"
)

var DEFAULT_NETWORK_BYTE uint8 = 0xef

/* Convert an integer into a WIF (base58 string) */
func MakeWIF(input string, networkByte uint8) (string, error) {
	/* network byte + key data + compressed flag + checksum */
	buf := make([]byte, 1+32+1+4)

	buf[0] = networkByte
	buf[33] = 0x01

	/* convert the input into a 32 byte integer, big endian */
	val := new(big.Int)
	val, ok := val.SetString(input, 10)
	if !ok {
		return "", fmt.Errorf("Input not an int '%s'", input)
	}

	valBuf := val.Bytes()
	if len(valBuf) > 32 {
		return "", fmt.Errorf("Input too big '%s'", input)
	}

	start := 32 - len(valBuf)
	copy(buf[1+start:], valBuf)

	/* compute the checksum + add to buffer result */
	h := sha256.New()
	h.Write(buf[:1+32+1])
	firstsha := h.Sum(nil)
	h = sha256.New()
	h.Write(firstsha)
	checksum := h.Sum(nil)

	copy(buf[34:], checksum[:4])

	/* find base58 of this */
	return base58.Encode(buf), nil
}

func MakeAddr(input string, hrp string) (string, error) {
	/* For now, only work on segwit */
	data, err := hex.DecodeString(input)
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", fmt.Errorf("Not a valid segwit scriptPubkey %s %d", input, len(data))
	}
	if data[0] != 0x00 || (len(data) != 34 && len(data) != 22) {
		return "", fmt.Errorf("Not a valid segwit scriptPubkey %s %d", input, len(data))
	}
	witver, err := bech32.ConvertBits(data[0:1], 8, 5, false)
	if err != nil {
		return "", err
	}
	program, err := bech32.ConvertBits(data[2:], 8, 5, true)
	if err != nil {
		return "", err
	}
	encoded, err := bech32.Encode(hrp, append(witver, program...))

	return encoded, err
}
