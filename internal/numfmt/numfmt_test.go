package numfmt

import "testing"

func TestFormatAddsThousandsSeparators(t *testing.T) {
	cases := map[int]string{
		0: "0", 7: "7", 412: "412", 999: "999",
		1000: "1,000", 1248: "1,248", 3072: "3,072", 34712: "34,712",
		1234567: "1,234,567", -4321: "-4,321",
	}
	for value, want := range cases {
		if got := Format(value); got != want {
			t.Errorf("Format(%d) = %q; want %q", value, got, want)
		}
	}
}
