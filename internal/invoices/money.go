// Package invoices defines canonical invoice values; it never uses float money.
package invoices

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
)

// Money represents an exact signed integer number of currency minor units.
type Money struct {
	MinorUnits int64
	Currency   string
}

const RoundingPolicyV1 = "money-v1"

var ErrInvalidDecimal = errors.New("invalid exact decimal")

// CurrencyExponent is intentionally small and explicit for the no-key demo.
// Adding a currency changes the accepted Stage 3 proposal contract and needs a
// decision/documentation update rather than a silent three-letter assumption.
func CurrencyExponent(currency string) (int, bool) {
	switch strings.ToUpper(strings.TrimSpace(currency)) {
	case "USD", "EUR", "GBP", "RUB":
		return 2, true
	case "JPY":
		return 0, true
	default:
		return 0, false
	}
}

// ParseExactDecimal accepts only an ASCII base-10 literal. It returns a Rat
// so callers never accidentally introduce binary floating-point rounding.
func ParseExactDecimal(value string) (*big.Rat, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, ErrInvalidDecimal
	}
	start := 0
	if value[0] == '-' {
		start = 1
	}
	if start == len(value) {
		return nil, ErrInvalidDecimal
	}
	dots := 0
	digits := 0
	for _, r := range value[start:] {
		switch {
		case r >= '0' && r <= '9':
			digits++
		case r == '.':
			dots++
			if dots > 1 {
				return nil, ErrInvalidDecimal
			}
		default:
			return nil, ErrInvalidDecimal
		}
	}
	if digits == 0 {
		return nil, ErrInvalidDecimal
	}
	parsed, ok := new(big.Rat).SetString(value)
	if !ok {
		return nil, ErrInvalidDecimal
	}
	return parsed, nil
}

// DecimalToMinorV1 implements the persisted money-v1 policy: nearest minor
// unit, ties to even. It returns an error instead of overflowing int64.
func DecimalToMinorV1(value string, exponent int) (int64, error) {
	rational, err := ParseExactDecimal(value)
	if err != nil {
		return 0, err
	}
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exponent)), nil)
	rational.Mul(rational, new(big.Rat).SetInt(scale))
	return roundRatToInt64V1(rational)
}

// MinorToDecimalV1 renders stored integer minor units as a canonical exact
// decimal for an editable review form. It is the inverse presentation shape
// of money-v1 storage; it does not use floating-point arithmetic.
func MinorToDecimalV1(minor int64, exponent int) string {
	if exponent == 0 {
		return fmt.Sprintf("%d", minor)
	}
	negative := minor < 0
	value := big.NewInt(minor)
	if negative {
		value.Abs(value)
	}
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exponent)), nil)
	whole, fraction := new(big.Int), new(big.Int)
	whole.QuoRem(value, scale, fraction)
	result := fmt.Sprintf("%s.%0*d", whole.String(), exponent, fraction.Int64())
	if negative {
		return "-" + result
	}
	return result
}

func roundRatToInt64V1(rational *big.Rat) (int64, error) {
	numerator, denominator := rational.Num(), rational.Denom()
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(numerator, denominator, remainder)
	absoluteRemainder := new(big.Int).Abs(remainder)
	comparison := new(big.Int).Lsh(absoluteRemainder, 1).Cmp(denominator)
	if comparison > 0 || (comparison == 0 && quotient.Bit(0) == 1) {
		if numerator.Sign() >= 0 {
			quotient.Add(quotient, big.NewInt(1))
		} else {
			quotient.Sub(quotient, big.NewInt(1))
		}
	}
	if !quotient.IsInt64() {
		return 0, ErrInvalidDecimal
	}
	return quotient.Int64(), nil
}

func NewMoney(minorUnits int64, currency string) (Money, error) {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if len(currency) != 3 {
		return Money{}, fmt.Errorf("currency must be a three-letter ISO code")
	}
	for _, r := range currency {
		if r < 'A' || r > 'Z' {
			return Money{}, fmt.Errorf("currency must contain only ASCII letters")
		}
	}
	return Money{MinorUnits: minorUnits, Currency: currency}, nil
}
