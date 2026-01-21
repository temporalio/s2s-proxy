package common

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

func TestIsRequestTranslationDisabled(t *testing.T) {
	tests := []struct {
		meta        map[string]string
		expDisabled bool
	}{
		{
			meta: nil,
		},
		{
			meta: map[string]string{},
		},
		{
			meta: map[string]string{RequestTranslationHeaderName: ""},
		},
		{
			meta: map[string]string{RequestTranslationHeaderName: "true"},
		},
		{
			meta: map[string]string{RequestTranslationHeaderName: "asdf"},
		},
		{
			meta:        map[string]string{RequestTranslationHeaderName: "false"},
			expDisabled: true,
		},
		{
			meta:        map[string]string{RequestTranslationHeaderName: "0"},
			expDisabled: true,
		},
	}

	for _, tc := range tests {
		name := fmt.Sprintf("%v", tc.meta)
		t.Run(name, func(t *testing.T) {
			ctx := metadata.NewIncomingContext(context.Background(), metadata.New(tc.meta))
			require.Equal(t, tc.expDisabled, IsRequestTranslationDisabled(ctx))
		})

	}

}

func TestGCD(t *testing.T) {
	cases := []struct{ a, b, exp int32 }{
		{0, 0, 0},
		{1, 0, 0},
		{0, 1, 0},

		{1, 1, 1},
		{2, 1, 1},
		{3, 1, 1},
		{4, 1, 1},
		{5, 1, 1},
		{6, 1, 1},
		{7, 1, 1},
		{8, 1, 1},
		{9, 1, 1},

		{2, 2, 2},
		{3, 2, 1},
		{4, 2, 2},
		{5, 2, 1},
		{6, 2, 2},
		{7, 2, 1},
		{8, 2, 2},
		{9, 2, 1},

		{3, 3, 3},
		{4, 3, 1},
		{5, 3, 1},
		{6, 3, 3},
		{7, 3, 1},
		{8, 3, 1},
		{9, 3, 3},

		{4, 4, 4},
		{5, 4, 1},
		{6, 4, 2},
		{7, 4, 1},
		{8, 4, 4},
		{9, 4, 1},

		{5, 5, 5},
		{6, 5, 1},
		{7, 5, 1},
		{8, 5, 1},
		{9, 5, 1},

		{36, 10, 2},
		{36, 11, 1},
		{36, 12, 12},
		{36, 14, 2},
		{36, 15, 3},
		{36, 16, 4},

		{4000, 1024, 32},
		{4000, 512, 32},
	}

	for _, tc := range cases {
		name := fmt.Sprintf("a=%d, b=%d", tc.a, tc.b)
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.exp, GCD(tc.a, tc.b))
		})

		// GCD(a, b) == GCD(b, a)
		name = fmt.Sprintf("b=%d, a=%d", tc.b, tc.a)
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.exp, GCD(tc.b, tc.a))
		})
	}
}

func TestLCM(t *testing.T) {
	cases := []struct{ a, b, exp int32 }{
		{0, 0, 0},
		{1, 0, 0},
		{0, 1, 0},

		{1, 1, 1},
		{2, 1, 2},
		{3, 1, 3},
		{4, 1, 4},
		{5, 1, 5},
		{6, 1, 6},
		{7, 1, 7},
		{8, 1, 8},
		{9, 1, 9},

		{2, 2, 2},
		{3, 2, 6},
		{4, 2, 4},
		{5, 2, 10},
		{6, 2, 6},
		{7, 2, 14},
		{8, 2, 8},
		{9, 2, 18},

		{3, 3, 3},
		{4, 3, 12},
		{5, 3, 15},
		{6, 3, 6},
		{7, 3, 21},
		{8, 3, 24},
		{9, 3, 9},

		{4, 4, 4},
		{5, 4, 20},
		{6, 4, 12},
		{7, 4, 28},
		{8, 4, 8},
		{9, 4, 36},

		{36, 10, 180},
		{36, 11, 396},
		{36, 12, 36},

		{4000, 1024, 128000},
		{4000, 512, 64000},
		{4000, 256, 32000},
	}

	for _, tc := range cases {
		name := fmt.Sprintf("a=%d, b=%d", tc.a, tc.b)
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.exp, LCM(tc.a, tc.b))
		})

		// LCM(a, b) == LCM(b, a)
		name = fmt.Sprintf("b=%d, a=%d", tc.b, tc.a)
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.exp, LCM(tc.b, tc.a))
		})
	}
}
