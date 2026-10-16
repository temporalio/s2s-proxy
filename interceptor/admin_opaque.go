package interceptor

import (
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type (
	// OpaqueObserver is told about every piece of opaque data met while sealing
	// admin traffic: data such as HSM and CHASM state that the receiving server
	// decodes itself, so it cannot be sealed. kind is [OpaqueKind] of path, and
	// is bounded, which path is not; label metrics with kind only.
	OpaqueObserver interface {
		// OpaquePassed reports data sent on unsealed because plaintext is allowed.
		OpaquePassed(kind, path string)
		// OpaqueRejected reports data that failed its message instead.
		OpaqueRejected(kind, path string)
	}

	nopObserver struct{}
)

// OpaqueKind classifies an opaque path reported by [VisitAdminPayloads] as
// "chasm", "hsm", "task", or "other". CHASM is checked first because CHASM
// paths run through mutable state too.
func OpaqueKind(path string) string {
	switch {
	case strings.Contains(path, "Chasm"):
		return "chasm"
	case strings.Contains(path, "StateMachine"):
		return "hsm"
	case strings.HasSuffix(path, "/Blob"), strings.HasSuffix(path, "ReplicationTask/Data"):
		return "task"
	default:
		return "other"
	}
}

// sealOpaque is the opaque handler for the sealing direction. Allowed, the data
// goes through as it is and obs hears about it. Otherwise the message fails,
// naming the path but never the data.
func sealOpaque(allow bool, obs OpaqueObserver) func(string, []byte) error {
	if obs == nil {
		obs = nopObserver{}
	}

	return func(path string, _ []byte) error {
		kind := OpaqueKind(path)
		if allow {
			obs.OpaquePassed(kind, path)
			return nil
		}

		obs.OpaqueRejected(kind, path)
		return status.Errorf(codes.FailedPrecondition,
			"cannot seal opaque %s data at %s; set encryption.allowOpaquePlaintext to send it unsealed", kind, path)
	}
}

// openOpaque is the opaque handler for the opening direction. Opaque data from
// the peer was never sealed, so there is nothing to do, and refusing it would
// block failback.
func openOpaque(string, []byte) error { return nil }

func (nopObserver) OpaquePassed(string, string)   {}
func (nopObserver) OpaqueRejected(string, string) {}
