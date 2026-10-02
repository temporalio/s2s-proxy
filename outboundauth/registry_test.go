package outboundauth

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/server/common/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

type testCredentials struct {
	metadata map[string]string
	err      error
	require  bool
}

func (c testCredentials) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	return c.metadata, c.err
}

func (c testCredentials) RequireTransportSecurity() bool {
	return c.require
}

func TestNewRegistryRejectsInvalidRegistrations(t *testing.T) {
	tests := []struct {
		name          string
		registrations []Registration
		want          string
	}{
		{name: "invalid name", registrations: []Registration{{Name: "Bad Name", Factory: FactoryFunc(nil)}}, want: "invalid"},
		{name: "nil factory", registrations: []Registration{{Name: "valid", Factory: nil}}, want: "no factory"},
		{name: "nil factory function", registrations: []Registration{{Name: "valid", Factory: FactoryFunc(nil)}}, want: "no factory"},
		{name: "duplicate", registrations: []Registration{{Name: "valid", Factory: testFactory()}, {Name: "valid", Factory: testFactory()}}, want: "duplicate"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewRegistry(test.registrations...)
			require.ErrorContains(t, err, test.want)
		})
	}
}

func TestRegistryBuild(t *testing.T) {
	properties := map[string]string{"audience": "frontend"}
	var received BuildRequest
	registry, err := NewRegistry(Registration{
		Name: "custom",
		Factory: FactoryFunc(func(request BuildRequest) (BuiltCredentials, error) {
			received = request
			request.Properties["audience"] = "changed"
			return BuiltCredentials{
				PerRPC:            testCredentials{metadata: map[string]string{"X-Custom-Auth": "value"}},
				OwnedMetadataKeys: []string{"X-Custom-Auth"},
			}, nil
		}),
	})
	require.NoError(t, err)

	built, err := registry.Build("custom", BuildRequest{
		Target: Target{
			ClusterConnection: "migration",
			Destination:       DestinationLocal,
			Address:           "frontend.example.invalid:7233",
			Transport:         "tcp",
		},
		Properties: properties,
	}, log.NewNoopLogger())
	require.NoError(t, err)
	assert.Equal(t, "frontend", properties["audience"])
	assert.Equal(t, DestinationLocal, received.Target.Destination)
	assert.Equal(t, []string{"x-custom-auth"}, built.OwnedMetadataKeys)
	assert.True(t, built.PerRPC.RequireTransportSecurity())

	metadataValues, err := built.PerRPC.GetRequestMetadata(t.Context())
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"x-custom-auth": "value"}, metadataValues)
}

func TestRegistryBuildRejectsInvalidResults(t *testing.T) {
	tests := []struct {
		name  string
		built BuiltCredentials
		want  string
	}{
		{name: "nil credentials", built: BuiltCredentials{OwnedMetadataKeys: []string{"authorization"}}, want: "nil credentials"},
		{name: "no keys", built: BuiltCredentials{PerRPC: testCredentials{}}, want: "no owned metadata"},
		{name: "duplicate keys", built: BuiltCredentials{PerRPC: testCredentials{}, OwnedMetadataKeys: []string{"Authorization", "authorization"}}, want: "duplicate"},
		{name: "invalid key", built: BuiltCredentials{PerRPC: testCredentials{}, OwnedMetadataKeys: []string{"bad key"}}, want: "invalid metadata"},
		{name: "grpc key", built: BuiltCredentials{PerRPC: testCredentials{}, OwnedMetadataKeys: []string{"grpc-timeout"}}, want: "reserved"},
		{name: "routing key", built: BuiltCredentials{PerRPC: testCredentials{}, OwnedMetadataKeys: []string{"temporal-client-shard-id"}}, want: "reserved"},
		{name: "principal key", built: BuiltCredentials{PerRPC: testCredentials{}, OwnedMetadataKeys: []string{"temporal-principal-name"}}, want: "reserved"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registry, err := NewRegistry(Registration{Name: "custom", Factory: FactoryFunc(func(BuildRequest) (BuiltCredentials, error) {
				return test.built, nil
			})})
			require.NoError(t, err)
			_, err = registry.Build("custom", BuildRequest{}, log.NewNoopLogger())
			require.ErrorContains(t, err, test.want)
		})
	}
}

func TestRegistryBuildErrors(t *testing.T) {
	registry, err := NewRegistry(Registration{Name: "broken", Factory: FactoryFunc(func(BuildRequest) (BuiltCredentials, error) {
		return BuiltCredentials{}, errors.New("failed")
	})})
	require.NoError(t, err)

	_, err = registry.Build("missing", BuildRequest{}, log.NewNoopLogger())
	require.ErrorContains(t, err, "unknown")
	_, err = registry.Build("broken", BuildRequest{}, log.NewNoopLogger())
	require.ErrorContains(t, err, "failed")
}

func TestValidatedCredentials(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]string
		err      error
		wantCode codes.Code
	}{
		{name: "empty result", wantCode: codes.Unauthenticated},
		{name: "empty value", metadata: map[string]string{"authorization": ""}, wantCode: codes.Unauthenticated},
		{name: "undeclared key", metadata: map[string]string{"other": "value"}, wantCode: codes.Internal},
		{name: "duplicate normalized key", metadata: map[string]string{"Authorization": "one", "authorization": "two"}, wantCode: codes.Internal},
		{name: "invalid value", metadata: map[string]string{"authorization": "value\nother"}, wantCode: codes.Internal},
		{name: "plain provider error", err: errors.New("secret detail"), wantCode: codes.Unavailable},
		{name: "unavailable provider", err: status.Error(codes.Unavailable, "secret detail"), wantCode: codes.Unavailable},
		{name: "deadline provider", err: status.Error(codes.DeadlineExceeded, "secret detail"), wantCode: codes.DeadlineExceeded},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			credentials := validatedCredentials{
				delegate:    testCredentials{metadata: test.metadata, err: test.err},
				ownedKeys:   map[string]struct{}{"authorization": {}},
				provider:    "test-provider",
				destination: DestinationRemote,
				logger:      log.NewNoopLogger(),
			}
			_, err := credentials.GetRequestMetadata(t.Context())
			require.Error(t, err)
			assert.Equal(t, test.wantCode, status.Code(err))
			assert.NotContains(t, err.Error(), "secret detail")
		})
	}
}

func TestValidatedCredentialsUsesContextStatus(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	credentials := validatedCredentials{
		delegate:    testCredentials{err: errors.New("failed")},
		ownedKeys:   map[string]struct{}{"authorization": {}},
		provider:    "test-provider",
		destination: DestinationRemote,
		logger:      log.NewNoopLogger(),
	}
	_, err := credentials.GetRequestMetadata(ctx)
	require.Error(t, err)
	assert.Equal(t, codes.Canceled, status.Code(err))
}

func testFactory() Factory {
	return FactoryFunc(func(BuildRequest) (BuiltCredentials, error) {
		return BuiltCredentials{PerRPC: testCredentials{}, OwnedMetadataKeys: []string{"authorization"}}, nil
	})
}

var _ credentials.PerRPCCredentials = testCredentials{}
