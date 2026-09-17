package extension

import (
	"context"
	"fmt"

	kms "github.com/temporalio/temporal-proxy/pkg/api/kms/v1"
	"github.com/temporalio/temporal-proxy/pkg/crypto"
	"google.golang.org/grpc"
)

var _ crypto.KEK = (*KMS)(nil)

// KMS wraps and unwraps data encryption keys on an extension server
// implementing api.kms.v1.EncryptionService. Only key material crosses the
// wire; payload plaintext never reaches the server.
//
// The id names the key this client addresses. It is recorded in every DEK the
// key wraps and is what selects the key again when unwrapping, so it must stay
// stable for as long as any sealed payload references it.
type KMS struct {
	id  string
	kms kms.EncryptionServiceClient
}

// NewKMS returns a KMS addressing the key named by id over cc. Several keys may
// live on one extension server and share a connection, so cc is not owned here.
func NewKMS(id string, cc grpc.ClientConnInterface) *KMS {
	return &KMS{
		id:  id,
		kms: kms.NewEncryptionServiceClient(cc),
	}
}

// Close is a no-op. The gRPC connection passed to NewKMS is owned by whoever
// dialed it, which remains responsible for closing it; a KEKRegistry closing
// this key must not tear down a connection it does not own and other keys are
// still using.
func (k *KMS) Close() error {
	return nil
}

// ID returns the key URI this client addresses.
func (k *KMS) ID() string {
	return k.id
}

// Encrypt wraps a DEK on the extension server, returning the ciphertext. The
// namespace is forwarded so the server can select a per-namespace wrapping key.
func (k *KMS) Encrypt(ctx context.Context, ns string, pt []byte) ([]byte, error) {
	res, err := k.kms.Encrypt(ctx, &kms.EncryptRequest{
		Namespace: ns,
		Plaintext: pt,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt DEK, id: %s, err: %w", k.id, err)
	}

	return res.Ciphertext, nil
}

// Decrypt unwraps a DEK previously produced by Encrypt. No namespace is sent:
// the server is expected to read the key it needs out of its own ciphertext.
func (k *KMS) Decrypt(ctx context.Context, ct []byte) ([]byte, error) {
	res, err := k.kms.Decrypt(ctx, &kms.DecryptRequest{
		Ciphertext: ct,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt DEK, id: %s, err: %w", k.id, err)
	}

	return res.Plaintext, nil
}
