package common

import (
	"context"
	"strconv"

	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc/metadata"
)

const (
	RequestTranslationHeaderName string = "s2s-request-translation"
)

func ServiceTag(sv string) tag.ZapTag {
	return tag.NewStringTag("service", sv)
}

// IsRequestTranslationDisabled returns true if `s2s-request-translation=false` in gRPC metadata.
func IsRequestTranslationDisabled(ctx context.Context) bool {
	values := metadata.ValueFromIncomingContext(ctx, RequestTranslationHeaderName)
	if len(values) == 0 {
		return false
	}
	enabled, err := strconv.ParseBool(values[0])
	if err != nil {
		return false
	}
	return !enabled
}
