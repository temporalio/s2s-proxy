package extension

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestNewKeyFuncResolvesAServerByHost(t *testing.T) {
	conns := Connections{"hsm": &stubConn{}}

	key, err := NewKeyFunc(conns)(t.Context(), "extension://hsm/replication")
	require.NoError(t, err)
	require.Equal(t, "extension://hsm/replication", key.ID())
}

// TestNewKeyFuncNormalizesTheURI matters because the ID is recorded in every DEK
// the key wraps: two spellings of one key have to resolve to one identity, or
// payloads sealed under one spelling cannot be opened under the other.
func TestNewKeyFuncNormalizesTheURI(t *testing.T) {
	conns := Connections{"hsm": &stubConn{}}

	key, err := NewKeyFunc(conns)(t.Context(), "EXTENSION://hsm/replication")
	require.NoError(t, err)
	require.Equal(t, "extension://hsm/replication", key.ID())
}

func TestNewKeyFuncErrors(t *testing.T) {
	cases := []struct {
		name  string
		conns Connections
		uri   string
		want  string
	}{
		{
			name:  "unknown server",
			conns: Connections{"hsm": &stubConn{}},
			uri:   "extension://vault/replication",
			want:  `unknown extension server "vault"`,
		},
		{
			name:  "server names are matched case-sensitively",
			conns: Connections{"hsm": &stubConn{}},
			uri:   "extension://HSM/replication",
			want:  `unknown extension server "HSM"`,
		},
		{
			name:  "no server named at all",
			conns: Connections{"hsm": &stubConn{}},
			uri:   "extension:///replication",
			want:  "must name an extension server",
		},
		{
			name:  "no connections configured",
			conns: nil,
			uri:   "extension://hsm/replication",
			want:  `unknown extension server "hsm"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewKeyFunc(tc.conns)(t.Context(), tc.uri)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestSchemeMatchesTheConfiguredScheme(t *testing.T) {
	require.Equal(t, "extension", Scheme)
}

type stubConn struct{}

func (*stubConn) Invoke(context.Context, string, any, any, ...grpc.CallOption) error {
	return nil
}

func (*stubConn) NewStream(
	context.Context,
	*grpc.StreamDesc,
	string,
	...grpc.CallOption,
) (grpc.ClientStream, error) {
	return nil, nil
}
