package bytesz

import (
	"fmt"
)

var prefix = []string{"B  ", "KiB", "MiB", "GiB", "TiB", "PiB", "YiB"}

func Format(byteSize uint64) string {
	for i := len(prefix) - 1; i >= 0; i-- {
		exponent := i
		oneUnit := uint64(1) << (exponent * 10)
		if byteSize >= uint64(oneUnit) {
			return fmt.Sprintf("%6.1f %s", (float64(byteSize) / float64(oneUnit)), prefix[i])
		}
	}

	return fmt.Sprintf("%6.0f B  ", float64(byteSize))
}
