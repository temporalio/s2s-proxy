package interceptor

import (
	"context"
	"errors"
	"fmt"

	"go.temporal.io/api/common/v1"
	"go.temporal.io/api/failure/v1"
	"go.temporal.io/api/proxy"
	"go.temporal.io/proxy/pkg/codec"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/temporalio/s2s-proxy/config"
)

type (
	// AdminEncryptor seals and opens the payloads in admin service traffic, which
	// carries replication. It is the admin counterpart of [Encryptor], with the
	// same vault and directions, but it walks admin messages with
	// [VisitAdminPayloads] and seals everything under
	// [config.ReplicationKeyNamespace]: admin messages do not name their
	// namespace, so there is nothing to key them by. They do carry namespace IDs,
	// per replication task, so with [AdminEncryptorConfig.Namespaces] payloads
	// are bound to the label of the namespace their task belongs to.
	//
	// Unary goes wherever a [grpc.UnaryClientInterceptor] is wanted and Stream
	// wherever a [grpc.StreamClientInterceptor] is.
	AdminEncryptor struct {
		Unary  grpc.UnaryClientInterceptor
		Stream grpc.StreamClientInterceptor
	}

	// AdminEncryptorConfig is everything [NewAdminEncryptor] needs. The embedded
	// EncryptorConfig means what it means for [NewEncryptor].
	AdminEncryptorConfig struct {
		EncryptorConfig
		// AllowOpaquePlaintext lets opaque data, such as HSM and CHASM state, cross
		// unsealed when sealing. Off, a message carrying any fails instead.
		AllowOpaquePlaintext bool
		// Observer hears about opaque data met while sealing. Optional.
		Observer OpaqueObserver
		// Namespaces resolves the namespace IDs admin messages carry to local
		// names, which Label then maps to the label payloads are bound to. Nil,
		// or a part of a message naming no ID, binds to the replication key's
		// name and compares nothing on open, which is also what a namespace that
		// fails to resolve gets unless StrictNamespaces is set.
		Namespaces NamespaceResolver
		// StrictNamespaces fails the payload, and so the call or stream message
		// carrying it, when its namespace ID does not resolve, instead of binding
		// it to the replication key's name. For a strict binding, under which
		// that fallback would be refused when read back.
		StrictNamespaces bool
		// NamespaceObserver hears about namespace IDs that failed to resolve,
		// whether the payload then failed or fell back. Optional.
		NamespaceObserver NamespaceObserver
		// VisitErrors hears about every unary call or stream failed because a
		// message could not be sealed or opened, with the full error, which the
		// caller of a unary call only gets a generic status for. Optional.
		VisitErrors VisitErrorObserver
	}

	// VisitErrorObserver hears about an admin call that failed because this
	// proxy could not seal or open one of its messages. Streams are told apart,
	// since whatever relays one already has the full error.
	VisitErrorObserver interface {
		VisitFailed(method string, err error)
		StreamVisitFailed(method string, err error)
	}

	// NamespaceResolver maps a namespace ID to the namespace's local name.
	NamespaceResolver interface {
		LocalName(ctx context.Context, id string) (string, error)
	}

	// NamespaceObserver hears about a namespace ID that failed to resolve, once
	// per payload it left unbound, which StrictNamespaces fails and otherwise
	// falls back to the replication key's name.
	NamespaceObserver interface {
		UnresolvedNamespace(id string, err error)
	}

	// adminVisit is one direction's work over an admin message. A nil adminVisit
	// does nothing, which is what a direction with no vault work gets.
	adminVisit struct {
		payloads *proxy.VisitPayloadsOptions
		opaque   func(path string, data []byte) error
		// details visits the payloads in a gRPC error's details, through the
		// upstream interceptor that already knows which detail types hold them.
		details grpc.UnaryClientInterceptor
		// failures visits failures alongside payloads, before them when sealing
		// and after when opening. Nil means no failure work in this direction.
		failures      *proxy.VisitFailuresOptions
		failuresAfter bool
		// failureDetails visits the failures in a gRPC error's details. Nil when
		// failures is.
		failureDetails grpc.UnaryClientInterceptor
	}
)

// NewAdminEncryptor returns the [AdminEncryptor] cfg describes. It takes the
// same three shapes as [NewEncryptor]: a vault with Enabled seals and opens, a
// vault alone only opens, and no vault does nothing. Enabled with no vault is
// refused. EncodeFailures means what it means for [NewEncryptor], and is
// refused without Enabled.
func NewAdminEncryptor(cfg AdminEncryptorConfig) (*AdminEncryptor, error) {
	if cfg.Enabled && cfg.Vault == nil {
		return nil, errors.New("proxy: encryption requires a vault")
	}

	if cfg.EncodeFailures && !cfg.Enabled {
		return nil, errors.New("proxy: failure encoding requires encryption")
	}

	var seal, open *adminVisit
	if cfg.Vault != nil {
		bind := namespaceBinder(cfg.Namespaces, cfg.Label, cfg.StrictNamespaces, cfg.NamespaceObserver)
		enc := []codecOpt{replicationKey(cipherOpt(cfg.Vault, bind, cfg.Binding))}

		var err error
		if open, err = newAdminVisit(visitPayloads(enc, codec.Chain.Decode), visitFailureOpts(restoreFailure), true, openOpaque); err != nil {
			return nil, err
		}

		if cfg.Enabled {
			var encode *proxy.VisitFailuresOptions
			if cfg.EncodeFailures {
				encode = visitFailureOpts(encodeFailure)
			}

			sealFn := sealUnlessSealed(alreadySealed(cfg.AlreadySealedEncodings))
			if seal, err = newAdminVisit(visitPayloads(enc, sealFn), encode, false, sealOpaque(cfg.AllowOpaquePlaintext, cfg.Observer)); err != nil {
				return nil, err
			}
		}
	}

	req, resp := seal, open
	if cfg.Reverse {
		req, resp = open, seal
	}

	return &AdminEncryptor{Unary: adminUnary(req, resp, cfg.VisitErrors), Stream: adminStream(req, resp, cfg.VisitErrors)}, nil
}

// newAdminVisit returns the adminVisit doing payloads, failures, and opaque in
// that order, or the reverse when failuresAfter. Whatever any of them fails
// with is marked a [VisitError] where it happens, so it is known for one
// wherever it surfaces, the error details of a failed call included.
func newAdminVisit(
	payloads *proxy.VisitPayloadsOptions,
	failures *proxy.VisitFailuresOptions,
	failuresAfter bool,
	opaque func(string, []byte) error,
) (*adminVisit, error) {
	payloads, failures, opaque = markPayloads(payloads), markFailures(failures), markOpaque(opaque)

	details, err := proxy.NewPayloadVisitorInterceptor(proxy.PayloadVisitorInterceptorOptions{Inbound: payloads})
	if err != nil {
		return nil, fmt.Errorf("failed to create admin encryption interceptor: %w", err)
	}

	v := &adminVisit{payloads: payloads, opaque: opaque, details: details, failures: failures, failuresAfter: failuresAfter}
	if failures != nil {
		if v.failureDetails, err = proxy.NewFailureVisitorInterceptor(proxy.FailureVisitorInterceptorOptions{Inbound: failures}); err != nil {
			return nil, fmt.Errorf("failed to create admin failure encryption interceptor: %w", err)
		}
	}

	return v, nil
}

// adminUnary seals or opens the request on its way out and the reply, or the
// error's details, on the way back. A failed visit on the way out returns
// before anything is sent.
//
// A request naming a namespace by ID scopes both visits to it, since replies
// such as raw history carry payloads but no ID of their own. IDs deeper in
// either message still take precedence for the parts they cover.
//
// A failed visit is reported to obs in full, then returned as its
// [VisitError], so the caller gets the generic status.
func adminUnary(req, resp *adminVisit, obs VisitErrorObserver) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, request, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if named, ok := request.(interface{ GetNamespaceId() string }); ok {
			if id := named.GetNamespaceId(); id != "" {
				ctx = withNamespaceID(ctx, id)
			}
		}

		failed := func(err error) error {
			visitErr, ok := asVisitError(err)
			if !ok {
				return err
			}
			if obs != nil {
				obs.VisitFailed(method, err)
			}

			return visitErr
		}

		if err := req.visit(ctx, request); err != nil {
			return failed(err)
		}

		if err := invoker(ctx, method, request, reply, cc, opts...); err != nil {
			return failed(resp.visitError(ctx, err))
		}

		return failed(resp.visit(ctx, reply))
	}
}

// adminStream wraps each stream so SendMsg runs the request direction and
// RecvMsg the response direction, one message at a time. With nothing to do in
// either direction the stream is returned as it is. A failed visit is reported
// to obs.
func adminStream(req, resp *adminVisit, obs VisitErrorObserver) grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		failed := func(err error) error {
			if _, ok := asVisitError(err); ok && obs != nil {
				obs.StreamVisitFailed(method, err)
			}

			return err
		}

		cs, err := streamer(ctx, desc, cc, method, opts...)
		if err != nil {
			return nil, failed(resp.visitError(ctx, err))
		}

		if req == nil && resp == nil {
			return cs, nil
		}

		return &adminClientStream{ClientStream: cs, ctx: ctx, send: req, recv: resp, failed: failed}, nil
	}
}

// adminClientStream visits every message crossing a stream. A failed visit is
// returned from SendMsg or RecvMsg like any stream error, which ends the
// stream for the caller; the message is never sent or handed over.
type adminClientStream struct {
	grpc.ClientStream
	ctx        context.Context
	send, recv *adminVisit
	// failed reports err if it is a failed visit, and returns it.
	failed func(err error) error
}

func (s *adminClientStream) SendMsg(m any) error {
	if err := s.send.visit(s.ctx, m); err != nil {
		return s.failed(surface(err))
	}

	return s.ClientStream.SendMsg(m)
}

func (s *adminClientStream) RecvMsg(m any) error {
	if err := s.ClientStream.RecvMsg(m); err != nil {
		return s.failed(surface(s.recv.visitError(s.ctx, err)))
	}

	return s.failed(surface(s.recv.visit(s.ctx, m)))
}

// VisitError is a message this proxy failed to seal or open, as opposed to an
// error from the call or stream carrying it. Something relaying a stream can
// tell the two apart with errors.As, and pass this one on.
//
// Error and Unwrap give the cause in full, for logging. GRPCStatus does not:
// whatever receives the status may be the peer, and the cause can name keys
// and namespaces, or carry codes from the systems behind them, such as the
// local frontend or a KMS. So it is always Internal, with a generic message.
type VisitError struct {
	err error
}

func (e *VisitError) Error() string { return e.err.Error() }

func (e *VisitError) Unwrap() error { return e.err }

func (e *VisitError) GRPCStatus() *status.Status {
	return status.New(codes.Internal, "s2s-proxy could not seal or open a replication message; see its logs")
}

// asVisitError returns the VisitError err is or wraps, if any.
func asVisitError(err error) (*VisitError, bool) {
	var visitErr *VisitError
	if !errors.As(err, &visitErr) {
		return nil, false
	}

	return visitErr, true
}

// surface returns the VisitError err wraps in its place, and any other err as
// it is. gRPC builds a status from the outermost error, taking a wrapped
// status's code but the wrapper's text, which would put the cause back in front
// of the caller.
func surface(err error) error {
	if visitErr, ok := asVisitError(err); ok {
		return visitErr
	}

	return err
}

// markVisitError returns err as a VisitError, unless it is nil or already one.
func markVisitError(err error) error {
	if _, ok := asVisitError(err); err == nil || ok {
		return err
	}

	return &VisitError{err: err}
}

// markPayloads returns a copy of opts whose visitor marks its errors.
func markPayloads(opts *proxy.VisitPayloadsOptions) *proxy.VisitPayloadsOptions {
	if opts == nil {
		return nil
	}

	marked, visitor := *opts, opts.Visitor
	marked.Visitor = func(ctx *proxy.VisitPayloadsContext, ps []*common.Payload) ([]*common.Payload, error) {
		out, err := visitor(ctx, ps)
		return out, markVisitError(err)
	}

	return &marked
}

// markFailures returns a copy of opts whose visitor marks its errors.
func markFailures(opts *proxy.VisitFailuresOptions) *proxy.VisitFailuresOptions {
	if opts == nil {
		return nil
	}

	marked, visitor := *opts, opts.Visitor
	marked.Visitor = func(ctx *proxy.VisitFailuresContext, f *failure.Failure) error {
		return markVisitError(visitor(ctx, f))
	}

	return &marked
}

// markOpaque returns opaque marking its errors.
func markOpaque(opaque func(string, []byte) error) func(string, []byte) error {
	return func(path string, data []byte) error {
		return markVisitError(opaque(path, data))
	}
}

// visit runs v over m when m is a proto message.
func (v *adminVisit) visit(ctx context.Context, m any) error {
	msg, ok := m.(proto.Message)
	if v == nil || !ok {
		return nil
	}

	return VisitAdminPayloads(ctx, msg, AdminVisitOptions{
		PayloadOptions: v.payloads,
		FailureOptions: v.failures,
		FailuresAfter:  v.failuresAfter,
		Opaque:         v.opaque,
	})
}

// visitError runs v over the payloads and failures in err's gRPC status
// details, in the same order as visit. An error that is not a status, io.EOF
// above all, comes back as it is.
func (v *adminVisit) visitError(ctx context.Context, err error) error {
	if v == nil {
		return err
	}

	run := func(i grpc.UnaryClientInterceptor, err error) error {
		// A visit that already failed is not the call's error to visit: its
		// status is the generic one, and building a new error from that would
		// lose the VisitError, and the cause with it.
		if _, failed := asVisitError(err); i == nil || failed {
			return err
		}

		return i(ctx, "", nil, nil, nil,
			func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error { return err })
	}

	if v.failuresAfter {
		return run(v.failureDetails, run(v.details, err))
	}

	return run(v.details, run(v.failureDetails, err))
}

// replicationKey seals and opens through opt under the replication key,
// whatever namespace the context names: admin traffic is keyed by it alone. The
// context's namespace is left for the binder, which may bind by it.
func replicationKey(opt codecOpt) codecOpt {
	return func(ctx context.Context, _ string) codec.Option {
		return opt(ctx, config.ReplicationKeyNamespace)
	}
}

// namespaceBinder binds each payload to label(name), name being the local name
// of the namespace the payload belongs to, or to the key namespace, the
// replication key's name, when that is unknown or there are no names. A nil
// label leaves the local name as the label.
//
// The namespace is the ID on the context, set for the task or request carrying
// the payload and resolved through names, or else the local name a server
// interceptor stamped for a request naming one. An ID names cannot resolve is
// reported to obs, then fails the payload when strict, and otherwise falls back
// to the key namespace.
func namespaceBinder(names NamespaceResolver, label func(string) string, strict bool, obs NamespaceObserver) binder {
	if names == nil {
		return nil
	}

	bound := func(local string) binding {
		if label == nil {
			return binding{label: local, check: true, ns: local}
		}

		return binding{label: label(local), check: true, ns: local}
	}

	return func(ctx context.Context, ns string) (binding, error) {
		id, ok := namespaceIDFrom(ctx)
		if !ok || id == "" {
			if stamped, _ := ctx.Value(NamespaceKey).(string); stamped != "" {
				return bound(stamped), nil
			}

			return binding{ns: ns}, nil
		}

		local, err := names.LocalName(ctx, id)
		if err != nil {
			if obs != nil {
				obs.UnresolvedNamespace(id, err)
			}
			if strict {
				return binding{}, fmt.Errorf("failed to resolve namespace ID %q: %w", id, err)
			}

			return binding{ns: ns}, nil
		}

		return bound(local), nil
	}
}
