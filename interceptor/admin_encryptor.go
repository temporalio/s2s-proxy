package interceptor

import (
	"context"
	"errors"
	"fmt"

	"github.com/temporalio/temporal-proxy/pkg/codec"
	"go.temporal.io/api/proxy"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	"github.com/temporalio/s2s-proxy/config"
)

type (
	// AdminEncryptor seals and opens the payloads in admin service traffic, which
	// carries replication. It is the admin counterpart of [Encryptor], with the
	// same vault and directions, but it walks admin messages with
	// [VisitAdminPayloads] and seals everything under
	// [config.ReplicationKeyNamespace]: admin messages do not name their
	// namespace, so there is nothing to key them by.
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
	}

	// adminVisit is one direction's work over an admin message. A nil adminVisit
	// does nothing, which is what a direction with no vault work gets.
	adminVisit struct {
		payloads *proxy.VisitPayloadsOptions
		opaque   func(path string, data []byte) error
		// details visits the payloads in a gRPC error's details, through the
		// upstream interceptor that already knows which detail types hold them.
		details grpc.UnaryClientInterceptor
	}
)

// NewAdminEncryptor returns the [AdminEncryptor] cfg describes. It takes the
// same three shapes as [NewEncryptor]: a vault with Enabled seals and opens, a
// vault alone only opens, and no vault does nothing. Enabled with no vault is
// refused.
func NewAdminEncryptor(cfg AdminEncryptorConfig) (*AdminEncryptor, error) {
	if cfg.Enabled && cfg.Vault == nil {
		return nil, errors.New("proxy: encryption requires a vault")
	}

	var seal, open *adminVisit
	if cfg.Vault != nil {
		enc := []codecOpt{func(ctx context.Context, ns string) codec.Option {
			return codec.WithCipher(&cipher{ctx: ctx, ns: ns, v: cfg.Vault})
		}}

		var err error
		if open, err = newAdminVisit(visitPayloads(enc, codec.Chain.Decode), openOpaque); err != nil {
			return nil, err
		}

		if cfg.Enabled {
			sealFn := sealUnlessSealed(alreadySealed(cfg.AlreadySealedEncodings))
			if seal, err = newAdminVisit(visitPayloads(enc, sealFn), sealOpaque(cfg.AllowOpaquePlaintext, cfg.Observer)); err != nil {
				return nil, err
			}
		}
	}

	req, resp := seal, open
	if cfg.Reverse {
		req, resp = open, seal
	}

	return &AdminEncryptor{Unary: adminUnary(req, resp), Stream: adminStream(req, resp)}, nil
}

func newAdminVisit(payloads *proxy.VisitPayloadsOptions, opaque func(string, []byte) error) (*adminVisit, error) {
	details, err := proxy.NewPayloadVisitorInterceptor(proxy.PayloadVisitorInterceptorOptions{Inbound: payloads})
	if err != nil {
		return nil, fmt.Errorf("failed to create admin encryption interceptor: %w", err)
	}

	return &adminVisit{payloads: payloads, opaque: opaque, details: details}, nil
}

// adminUnary seals or opens the request on its way out and the reply, or the
// error's details, on the way back. A failed visit on the way out returns
// before anything is sent.
func adminUnary(req, resp *adminVisit) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, request, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if err := req.visit(ctx, request); err != nil {
			return err
		}

		if err := invoker(ctx, method, request, reply, cc, opts...); err != nil {
			return resp.visitError(ctx, err)
		}

		return resp.visit(ctx, reply)
	}
}

// adminStream wraps each stream so SendMsg runs the request direction and
// RecvMsg the response direction, one message at a time. With nothing to do in
// either direction the stream is returned as it is.
func adminStream(req, resp *adminVisit) grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		cs, err := streamer(ctx, desc, cc, method, opts...)
		if err != nil {
			return nil, resp.visitError(ctx, err)
		}

		if req == nil && resp == nil {
			return cs, nil
		}

		return &adminClientStream{ClientStream: cs, ctx: ctx, send: req, recv: resp}, nil
	}
}

// adminClientStream visits every message crossing a stream. A failed visit is
// returned from SendMsg or RecvMsg like any stream error, which ends the
// stream for the caller; the message is never sent or handed over.
type adminClientStream struct {
	grpc.ClientStream
	ctx        context.Context
	send, recv *adminVisit
}

func (s *adminClientStream) SendMsg(m any) error {
	if err := s.send.visit(s.ctx, m); err != nil {
		return err
	}

	return s.ClientStream.SendMsg(m)
}

func (s *adminClientStream) RecvMsg(m any) error {
	if err := s.ClientStream.RecvMsg(m); err != nil {
		return s.recv.visitError(s.ctx, err)
	}

	return s.recv.visit(s.ctx, m)
}

// visit runs v over m when m is a proto message. The replication namespace is
// put on the visit's context, replacing any a server interceptor stamped, so
// [visitPayloads] seals under the replication key.
func (v *adminVisit) visit(ctx context.Context, m any) error {
	msg, ok := m.(proto.Message)
	if v == nil || !ok {
		return nil
	}

	ctx = context.WithValue(ctx, NamespaceKey, config.ReplicationKeyNamespace)
	return VisitAdminPayloads(ctx, msg, AdminVisitOptions{Payloads: v.payloads, Opaque: v.opaque})
}

// visitError runs v over the payloads in err's gRPC status details. An error
// that is not a status, io.EOF above all, comes back as it is.
func (v *adminVisit) visitError(ctx context.Context, err error) error {
	if v == nil {
		return err
	}

	ctx = context.WithValue(ctx, NamespaceKey, config.ReplicationKeyNamespace)
	return v.details(ctx, "", nil, nil, nil,
		func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error { return err })
}
