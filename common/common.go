package common

import (
	"context"
	"strconv"
	"strings"

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

func IsInvalidUTF8Error(err error) bool {
	// Unfortunately, there's not a single error type we can import.
	// This matches the two different 'invalid utf-8' errors that could occur.
	//
	// https://github.com/protocolbuffers/protobuf-go/blob/8e8926ef675d99b1c9612f5d008f4dc803839f7a/internal/impl/codec_field.go#L19
	// https://github.com/temporalio/temporal/blob/4151e25df8096ca254b79518c1eb7fc125871756/common/utf8validator/validate.go#L73
	return err != nil &&
		strings.Contains(strings.ToLower(err.Error()), "invalid utf-8")
}
