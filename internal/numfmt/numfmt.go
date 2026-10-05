// Package numfmt 统一界面与日志里的数字格式：行数一律带千分位（1248 → 1,248）。
package numfmt

import (
	"strconv"
	"strings"
)

// Format 给整数加千分位；负数保留符号（行数不会为负，这里只是不写坏）。
func Format(value int) string {
	digits := strconv.Itoa(value)
	sign := ""
	if strings.HasPrefix(digits, "-") {
		sign, digits = "-", digits[1:]
	}
	var builder strings.Builder
	builder.WriteString(sign)
	for index, digit := range digits {
		if index > 0 && (len(digits)-index)%3 == 0 {
			builder.WriteByte(',')
		}
		builder.WriteRune(digit)
	}
	return builder.String()
}
