package outboundauth

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestBearerCredentials(t *testing.T) {
	credentials := NewBearerCredentials(TokenProviderFunc(func(context.Context) (string, error) {
		return "opaque-token", nil
	}))

	metadataValues, err := credentials.GetRequestMetadata(t.Context())
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"authorization": "Bearer opaque-token"}, metadataValues)
	assert.True(t, credentials.RequireTransportSecurity())
}

func TestBearerCredentialsFailures(t *testing.T) {
	providerError := errors.New("provider failed")
	tests := []struct {
		name     string
		token    string
		err      error
		wantCode codes.Code
	}{
		{name: "empty", wantCode: codes.Unauthenticated},
		{name: "provider error", err: providerError, wantCode: codes.Unknown},
		{name: "line feed", token: "opaque\ntoken", wantCode: codes.Internal},
		{name: "carriage return", token: "opaque\rtoken", wantCode: codes.Internal},
		{name: "delete", token: "opaque\x7ftoken", wantCode: codes.Internal},
		{name: "prefixed", token: "Bearer opaque-token", wantCode: codes.Internal},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			credentials := NewBearerCredentials(TokenProviderFunc(func(context.Context) (string, error) {
				return test.token, test.err
			}))
			_, err := credentials.GetRequestMetadata(t.Context())
			require.Error(t, err)
			if test.err != nil {
				assert.ErrorIs(t, err, test.err)
				return
			}
			assert.Equal(t, test.wantCode, status.Code(err))
			if test.token != "" {
				assert.NotContains(t, err.Error(), test.token)
			}
		})
	}
}

func TestBearerCredentialsRejectsNilProvider(t *testing.T) {
	credentials := NewBearerCredentials(nil)
	_, err := credentials.GetRequestMetadata(t.Context())
	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestBearerCredentialsPropagatesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	credentials := NewBearerCredentials(TokenProviderFunc(func(ctx context.Context) (string, error) {
		return "", ctx.Err()
	}))
	_, err := credentials.GetRequestMetadata(ctx)
	require.ErrorIs(t, err, context.Canceled)
}

func TestBearerCredentialsUsesFreshTokens(t *testing.T) {
	var token atomic.Value
	token.Store("first")
	credentials := NewBearerCredentials(TokenProviderFunc(func(context.Context) (string, error) {
		return token.Load().(string), nil
	}))

	first, err := credentials.GetRequestMetadata(t.Context())
	require.NoError(t, err)
	token.Store("second")
	second, err := credentials.GetRequestMetadata(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "Bearer first", first["authorization"])
	assert.Equal(t, "Bearer second", second["authorization"])
}

func TestBearerCredentialsConcurrentUse(t *testing.T) {
	var calls atomic.Int32
	credentials := NewBearerCredentials(TokenProviderFunc(func(context.Context) (string, error) {
		calls.Add(1)
		return "opaque-token", nil
	}))
	var waitGroup sync.WaitGroup
	failures := make(chan error, 100)
	for range 100 {
		waitGroup.Go(func() {
			metadataValues, err := credentials.GetRequestMetadata(t.Context())
			if err != nil {
				failures <- err
				return
			}
			if metadataValues["authorization"] != "Bearer opaque-token" {
				failures <- fmt.Errorf("unexpected authorization metadata %q", metadataValues["authorization"])
			}
		})
	}
	waitGroup.Wait()
	close(failures)
	for err := range failures {
		require.NoError(t, err)
	}
	assert.Equal(t, int32(100), calls.Load())
}
