package proxy

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/server/client/history"
	"google.golang.org/grpc/metadata"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/failoverversion"
)

func TestTranslateClusterShardMetadata(t *testing.T) {
	t.Parallel()
	mapping, err := failoverversion.NewMapping(config.FailoverVersionTranslationConfig{
		FVI: config.IntMapping{Local: 105, Remote: 100},
		IFV: config.IFVTranslationConfig{Mappings: []config.IntMapping{
			{Local: 1, Remote: 11},
			{Local: 2, Remote: 12},
		}},
	})
	require.NoError(t, err)
	original := metadata.Pairs(
		history.MetadataKeyClientClusterID, "1",
		history.MetadataKeyServerClusterID, "2",
		"unrelated", "preserved",
	)
	translated, err := translateClusterShardMetadata(original, mapping.LocalToRemote())
	require.NoError(t, err)
	require.Equal(t, []string{"11"}, translated.Get(history.MetadataKeyClientClusterID))
	require.Equal(t, []string{"12"}, translated.Get(history.MetadataKeyServerClusterID))
	require.Equal(t, []string{"preserved"}, translated.Get("unrelated"))
	require.Equal(t, []string{"1"}, original.Get(history.MetadataKeyClientClusterID))
}

func TestTranslateClusterShardMetadataRejectsInvalidIDs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		values []string
	}{
		{"missing", nil},
		{"duplicate", []string{"1", "2"}},
		{"empty", []string{""}},
		{"whitespace", []string{" 1"}},
		{"signed", []string{"+1"}},
		{"zero", []string{"0"}},
		{"negative", []string{"-1"}},
		{"overflow", []string{"2147483648"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			md := metadata.MD{
				history.MetadataKeyClientClusterID: tc.values,
				history.MetadataKeyServerClusterID: []string{"1"},
			}
			_, err := translateClusterShardMetadata(md, identityClusterIDTranslator{})
			var translationErr *failoverversion.TranslationError
			require.Error(t, err)
			require.True(t, errors.As(err, &translationErr))
			require.Equal(t, failoverversion.ErrorInvalidClusterID, translationErr.Code)
		})
	}
}

type identityClusterIDTranslator struct{}

func (identityClusterIDTranslator) TranslateLaneInt32(value int32) (int32, error) { return value, nil }
