package domain

import "math"

// Money represents an amount of VND as a whole number of minor-unit-free
// currency units (VND has no minor unit). It is always int64 — never a
// float — to avoid rounding errors in financial calculations.
type Money int64

// IsPositive reports whether m is strictly greater than zero. Debit and
// credit operations require a positive amount.
func (m Money) IsPositive() bool {
	return m > 0
}

// addOverflows reports whether a+b would overflow int64.
func addOverflows(a, b Money) bool {
	if b > 0 && a > math.MaxInt64-b {
		return true
	}
	if b < 0 && a < math.MinInt64-b {
		return true
	}
	return false
}
