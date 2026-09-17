package extension

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	kms "github.com/temporalio/temporal-proxy/pkg/api/kms/v1"
	"google.golang.org/grpc"
)

func TestKMSID(t *testing.T) {
	k := &KMS{id: "extension://hsm/replication"}

	require.Equal(t, "extension://hsm/replication", k.ID())
}

// TestKMSCloseDoesNotOwnTheConnection pins the behavior a KEKRegistry relies on:
// closing a key must not disturb the connection, which is shared with every
// other key on the same extension server.
func TestKMSCloseDoesNotOwnTheConnection(t *testing.T) {
	require.NoError(t, (&KMS{id: "extension://hsm/replication"}).Close())
}

func TestKMSEncrypt(t *testing.T) {
	var got *kms.EncryptRequest
	k := &KMS{id: "extension://hsm/replication", kms: &stubClient{
		encrypt: func(req *kms.EncryptRequest) (*kms.EncryptResponse, error) {
			got = req
			return &kms.EncryptResponse{Ciphertext: []byte("wrapped")}, nil
		},
	}}

	ct, err := k.Encrypt(context.Background(), "tenant-a", []byte("dek"))
	require.NoError(t, err)
	require.Equal(t, []byte("wrapped"), ct)
	require.Equal(t, "tenant-a", got.Namespace)
	require.Equal(t, []byte("dek"), got.Plaintext)
}

func TestKMSDecrypt(t *testing.T) {
	var got *kms.DecryptRequest
	k := &KMS{id: "extension://hsm/replication", kms: &stubClient{
		decrypt: func(req *kms.DecryptRequest) (*kms.DecryptResponse, error) {
			got = req
			return &kms.DecryptResponse{Plaintext: []byte("dek")}, nil
		},
	}}

	pt, err := k.Decrypt(context.Background(), []byte("wrapped"))
	require.NoError(t, err)
	require.Equal(t, []byte("dek"), pt)
	require.Equal(t, []byte("wrapped"), got.Ciphertext)
}

func TestKMSEncryptErrorNamesTheKey(t *testing.T) {
	k := &KMS{id: "extension://hsm/replication", kms: &stubClient{
		encrypt: func(*kms.EncryptRequest) (*kms.EncryptResponse, error) {
			return nil, errors.New("hsm unavailable")
		},
	}}

	_, err := k.Encrypt(context.Background(), "tenant-a", []byte("dek"))
	require.ErrorContains(t, err, "extension://hsm/replication")
	require.ErrorContains(t, err, "hsm unavailable")
}

func TestKMSDecryptErrorNamesTheKey(t *testing.T) {
	k := &KMS{id: "extension://hsm/replication", kms: &stubClient{
		decrypt: func(*kms.DecryptRequest) (*kms.DecryptResponse, error) {
			return nil, errors.New("hsm unavailable")
		},
	}}

	_, err := k.Decrypt(context.Background(), []byte("wrapped"))
	require.ErrorContains(t, err, "extension://hsm/replication")
	require.ErrorContains(t, err, "hsm unavailable")
}

type stubClient struct {
	encrypt func(*kms.EncryptRequest) (*kms.EncryptResponse, error)
	decrypt func(*kms.DecryptRequest) (*kms.DecryptResponse, error)
}

func (s *stubClient) Encrypt(
	_ context.Context,
	in *kms.EncryptRequest,
	_ ...grpc.CallOption,
) (*kms.EncryptResponse, error) {
	return s.encrypt(in)
}

func (s *stubClient) Decrypt(
	_ context.Context,
	in *kms.DecryptRequest,
	_ ...grpc.CallOption,
) (*kms.DecryptResponse, error) {
	return s.decrypt(in)
}
