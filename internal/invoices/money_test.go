package invoices

import "testing"

func TestNewMoneyRequiresCanonicalCurrency(t *testing.T) {
	money, err := NewMoney(1234, " eur ")
	if err != nil || money.Currency != "EUR" || money.MinorUnits != 1234 {
		t.Fatalf("NewMoney() = %#v, %v", money, err)
	}
	if _, err := NewMoney(1, "EURO"); err == nil {
		t.Fatal("NewMoney accepted invalid currency")
	}
}

func TestMinorToDecimalV1(t *testing.T) {
	for _, test := range []struct {
		minor    int64
		exponent int
		want     string
	}{
		{2400, 2, "24.00"}, {-5, 2, "-0.05"}, {42, 0, "42"}, {-9223372036854775808, 2, "-92233720368547758.08"},
	} {
		if got := MinorToDecimalV1(test.minor, test.exponent); got != test.want {
			t.Fatalf("MinorToDecimalV1(%d, %d) = %q, want %q", test.minor, test.exponent, got, test.want)
		}
	}
}
