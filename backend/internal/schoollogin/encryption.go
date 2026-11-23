package schoollogin

import (
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

type loginKey struct {
	exponent *big.Int
	modulus  *big.Int
	prefix   string
}

var keyFunction = regexp.MustCompile(`function\s+(_E|_M|_DK)\s*\(\s*\)\s*\{\s*return\s+("(?:[^"\\]|\\x[0-9a-fA-F]{2})*")\s*;?\s*\}`)

func parseLoginKey(script string) (loginKey, error) {
	values := make(map[string]string, 3)
	for _, match := range keyFunction.FindAllStringSubmatch(script, -1) {
		value, err := strconv.Unquote(match[2])
		if err != nil {
			return loginKey{}, fmt.Errorf("安全脚本的 %s 格式异常", match[1])
		}
		values[match[1]] = value
	}

	exponent, exponentOK := new(big.Int).SetString(values["_E"], 16)
	modulus, modulusOK := new(big.Int).SetString(values["_M"], 16)
	if !exponentOK || exponent.Sign() <= 0 || !modulusOK || modulus.Sign() <= 0 || modulus.BitLen() <= 16 || values["_DK"] == "" {
		return loginKey{}, fmt.Errorf("安全脚本缺少有效的 RSA 参数")
	}
	return loginKey{
		exponent: exponent,
		modulus:  modulus,
		prefix:   values["_DK"],
	}, nil
}

func (key loginKey) encrypt(value string) string {
	source := encodeURIComponent(key.prefix + value)
	// RSAUtils reserves the highest 16-bit word and packs each plaintext block little-endian.
	blockSize := 2 * ((key.modulus.BitLen() - 1) / 16)
	blocks := make([]string, 0, (len(source)+blockSize-1)/blockSize)
	block := make([]byte, blockSize)
	for len(source) > 0 {
		size := min(len(source), blockSize)
		clear(block)
		for i := range size {
			block[blockSize-1-i] = source[i]
		}
		source = source[size:]

		number := new(big.Int).SetBytes(block)
		hex := number.Exp(number, key.exponent, key.modulus).Text(16)
		// RSAUtils renders each word with four hex digits, including the leading word.
		hex = strings.Repeat("0", (4-len(hex)%4)%4) + hex
		blocks = append(blocks, hex)
	}
	return strings.Join(blocks, " ")
}

func encodeURIComponent(value string) string {
	const hex = "0123456789ABCDEF"
	var escaped strings.Builder
	for i := range len(value) {
		b := value[i]
		if 'a' <= b && b <= 'z' || 'A' <= b && b <= 'Z' || '0' <= b && b <= '9' ||
			strings.IndexByte("-_.!~*'()", b) >= 0 {
			escaped.WriteByte(b)
			continue
		}
		escaped.WriteByte('%')
		escaped.WriteByte(hex[b>>4])
		escaped.WriteByte(hex[b&15])
	}
	return escaped.String()
}
