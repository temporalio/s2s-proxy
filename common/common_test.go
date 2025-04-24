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
