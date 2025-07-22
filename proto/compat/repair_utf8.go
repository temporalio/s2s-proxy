package compat

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/temporalio/s2s-proxy/common"
	failure122 "github.com/temporalio/s2s-proxy/proto/1_22/api/failure/v1"
)

const (
	maxFailureDepth      = 10
	replacementCharacter = string(utf8.RuneError)
)

// In old versions of Temporal, it was possible that certain history events could
// be written with invalid UTF-8. This function will automatically repair the invalid
// UTF-8 string by deserializing, replacing the invalid UTF-8 characters, and then
// reserializating. We need to do the initial deserialization with an old version of
// protos that does not error on invalid UTF-8 strings.
func convertAndRepairInvalidUTF8(data []byte, v any) error {
	vMarshaler, ok := v.(common.Marshaler)
	if !ok {
		return fmt.Errorf("could not cast %T as a marshaler", v)
	}

	// Convert to the "same" type in the old protos.
	msg122, ok := adminConvertTo122(v)
	if !ok {
		msg122, ok = frontendConvertTo122(v)
	}
	if !ok || msg122 == nil {
		return fmt.Errorf("could not convert %T to gogo-based protobuf type", v)
	}

	if err := msg122.Unmarshal(data); err != nil {
		return fmt.Errorf("could not unmarshal data into %T: %w", msg122, err)
	}

	changed, err := repairInvalidUTF8(msg122)
	if err != nil {
		return err
	}
	if !changed {
		return fmt.Errorf("nothing was repaired in type %T", msg122)
	}

	repaired, err := msg122.Marshal()
	if err != nil {
		return fmt.Errorf("failed to re-marshal message %T after repair: %w", msg122, err)
	}
	if err := vMarshaler.Unmarshal(repaired); err != nil {
		return fmt.Errorf("failed to re-unmarshal message %T after repair: %w", v, err)
	}
	return nil
}

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
