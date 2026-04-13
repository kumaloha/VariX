package twitter

import (
	"math"
	"regexp"
	"strconv"
)

var stripSyndicationNoise = regexp.MustCompile(`(0+|\.)`)

func SyndicationToken(tweetID string) string {
	if tweetID == "" {
		return ""
	}
	id, err := strconv.ParseFloat(tweetID, 64)
	if err != nil {
		return ""
	}

	val := (id / 1e15) * math.Pi
	const chars = "0123456789abcdefghijklmnopqrstuvwxyz"

	integer := int(val)
	frac := val - float64(integer)
	parts := make([]byte, 0, 16)
	if integer == 0 {
		parts = append(parts, '0')
	}
	for integer > 0 {
		parts = append([]byte{chars[integer%36]}, parts...)
		integer /= 36
	}
	if frac > 0 {
		parts = append(parts, '.')
		for i := 0; i < 10; i++ {
			frac *= 36
			digit := int(frac)
			parts = append(parts, chars[digit])
			frac -= float64(digit)
		}
	}
	return stripSyndicationNoise.ReplaceAllString(string(parts), "")
}
