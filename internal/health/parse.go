package health

import "strconv"

func parseUint64(s string) uint64 {
	v, _ := strconv.ParseUint(s, 10, 64)
	return v
}

func parseFloat64(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}
