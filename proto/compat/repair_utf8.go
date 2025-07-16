package compat

import (
	"fmt"
	"strings"
	"unicode/utf8"

	failure122 "github.com/temporalio/s2s-proxy/proto/1_22/api/failure/v1"
)

const (
	maxFailureDepth    = 10
	replacementCharacter = string(utf8.RuneError)
)

func repairInvalidUTF8InFailure(failure *failure122.Failure) (bool, error) {
	changed := false
	for count := 0; failure != nil && count < maxFailureDepth; count++ {
		if !utf8.ValidString(failure.GetMessage()) {
			failure.Message = strings.ToValidUTF8(failure.Message, replacementCharacter)
			changed = true
		}
		failure = failure.GetCause()
	}
	if failure != nil {
		return changed, fmt.Errorf("reached maximum failure chain depth")
	}
	return changed, nil
}
