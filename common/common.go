package common

import (
	"go.temporal.io/server/common/log/tag"
)

func ServiceTag(sv string) tag.ZapTag {
	return tag.NewStringTag("service", sv)
}

// GCD calulates the greatest common divisor using the Euclidean algorithm.
//
// https://en.wikipedia.org/wiki/Euclidean_algorithm#Implementations
func GCD(a, b int32) int32 {
	if a == 0 || b == 0 {
		return 0
	}
	if a > b {
		a, b = b, a
	}

	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// LCM calcuates the least common multiple of a and b.
//
// https://en.wikipedia.org/wiki/Least_common_multiple#Using_the_greatest_common_divisor
func LCM(a, b int32) int32 {
	if a == 0 || b == 0 {
		return 0
	}
	return a * b / GCD(a, b)
}
