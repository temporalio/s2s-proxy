package failoverversion

import (
	"errors"
	"math"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/temporalio/s2s-proxy/config"
)

func testMapping(t testing.TB) *Mapping {
	t.Helper()
	mapping, err := NewMapping(config.FailoverVersionTranslationConfig{
		FVI: config.IntMapping{Local: 105, Remote: 100},
		IFV: config.IFVTranslationConfig{Mappings: []config.IntMapping{
			{Local: 1, Remote: 1},
			{Local: 2, Remote: 2},
			{Local: 105, Remote: 100},
		}},
	})
	require.NoError(t, err)
	return mapping
}

func TestTranslateVersionRoundTrip(t *testing.T) {
	t.Parallel()
	mapping := testMapping(t)
	forward := mapping.LocalToRemote()
	inverse := mapping.RemoteToLocal()
	for epoch := int64(0); epoch < 10_000; epoch += 97 {
		for _, lane := range []int64{1, 2, 105} {
			input := epoch*105 + lane
			translated, err := forward.TranslateVersion(input)
			require.NoError(t, err)
			roundTrip, err := inverse.TranslateVersion(translated)
			require.NoError(t, err)
			require.Equal(t, input, roundTrip)
		}
	}
	zero, err := forward.TranslateVersion(0)
	require.NoError(t, err)
	require.Zero(t, zero)
}

func TestTranslateScalarKinds(t *testing.T) {
	t.Parallel()
	forward := testMapping(t).LocalToRemote()

	fvi, err := forward.TranslateFailoverVersionIncrement(105)
	require.NoError(t, err)
	require.Equal(t, int64(100), fvi)
	_, err = forward.TranslateFailoverVersionIncrement(100)
	requireErrorCode(t, err, ErrorUnexpectedFVI)

	lane, err := forward.TranslateLane(105)
	require.NoError(t, err)
	require.Equal(t, int64(100), lane)
	zero, err := forward.TranslateLaneInt32(0)
	require.NoError(t, err)
	require.Zero(t, zero)
	_, err = forward.TranslateLane(3)
	requireErrorCode(t, err, ErrorUnknownLane)
}

func TestTranslateScalarFailureMatrix(t *testing.T) {
	t.Parallel()
	forward := testMapping(t).LocalToRemote()
	inverse := testMapping(t).RemoteToLocal()
	cases := []struct {
		name string
		call func() error
		code ErrorCode
	}{
		{name: "negative version forward", call: func() error { _, err := forward.TranslateVersion(-1); return err }, code: ErrorUnknownLane},
		{name: "unknown version lane forward", call: func() error { _, err := forward.TranslateVersion(3); return err }, code: ErrorUnknownLane},
		{name: "unknown version lane inverse", call: func() error { _, err := inverse.TranslateVersion(3); return err }, code: ErrorUnknownLane},
		{name: "negative lane", call: func() error { _, err := forward.TranslateLane(-1); return err }, code: ErrorUnknownLane},
		{name: "unexpected fvi inverse", call: func() error { _, err := inverse.TranslateFailoverVersionIncrement(105); return err }, code: ErrorUnexpectedFVI},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { requireErrorCode(t, tc.call(), tc.code) })
	}

	zero, err := inverse.TranslateLane(0)
	require.NoError(t, err)
	require.Zero(t, zero)
}

func TestTranslateVersionPreservesOrderAndEpoch(t *testing.T) {
	t.Parallel()
	forward := testMapping(t).LocalToRemote()
	previous := int64(0)
	for epoch := int64(0); epoch < 1_000; epoch++ {
		for _, lane := range []int64{1, 2, 105} {
			translated, err := forward.TranslateVersion(epoch*105 + lane)
			require.NoError(t, err)
			require.Greater(t, translated, previous)
			require.Equal(t, epoch, (translated-1)/100)
			previous = translated
		}
	}
}

func TestTranslateVersionOverflowAndRedaction(t *testing.T) {
	t.Parallel()
	forward := testMapping(t).LocalToRemote()
	_, err := forward.TranslateVersion(math.MaxInt64)
	require.Error(t, err)
	var translationErr *TranslationError
	require.True(t, errors.As(err, &translationErr))
	require.NotContains(t, err.Error(), strconv.FormatInt(math.MaxInt64, 10))

	withField := WithField(err, "unsafe/type value", "items[secret-value]")
	require.NotSame(t, translationErr, withField)
	require.Equal(t, "unsafe_type_value", withField.Type)
	require.NotContains(t, withField.Error(), strconv.FormatInt(math.MaxInt64, 10))
}

func TestTranslationErrorCodesRemainBoundedAndRedacted(t *testing.T) {
	t.Parallel()
	codes := []ErrorCode{
		ErrorUnknownLane, ErrorUnexpectedFVI, ErrorOverflow, ErrorInvalidClusterID,
		ErrorDecodeFailed, ErrorEncodeFailed, ErrorUnsupported, ErrorStatusDetail,
	}
	for _, code := range codes {
		t.Run(string(code), func(t *testing.T) {
			err := NewTranslationError(code, LocalToRemote, "unsafe/type secret-id", "items[secret-value]", errors.New("raw version 9223372036854775807"))
			require.Contains(t, err.Error(), "code="+string(code))
			require.NotContains(t, err.Error(), "9223372036854775807")
			require.NotContains(t, err.Error(), "secret-value")
			require.ErrorContains(t, err.Unwrap(), "9223372036854775807")
		})
	}
}

func TestMappingConcurrentUse(t *testing.T) {
	t.Parallel()
	forward := testMapping(t).LocalToRemote()
	result := make(chan error, 64)
	for i := 0; i < cap(result); i++ {
		go func(epoch int64) {
			_, err := forward.TranslateVersion(epoch*105 + 1)
			result <- err
		}(int64(i))
	}
	for i := 0; i < cap(result); i++ {
		require.NoError(t, <-result)
	}
}

func TestMappingDescriptorIsCanonicalAndRedacted(t *testing.T) {
	t.Parallel()
	cfg := config.FailoverVersionTranslationConfig{
		FVI: config.IntMapping{Local: 105, Remote: 100},
		IFV: config.IFVTranslationConfig{Mappings: []config.IntMapping{
			{Local: 105, Remote: 100}, {Local: 2, Remote: 2}, {Local: 1, Remote: 1},
		}},
	}
	first, err := NewMapping(cfg)
	require.NoError(t, err)
	cfg.IFV.Mappings[0], cfg.IFV.Mappings[2] = cfg.IFV.Mappings[2], cfg.IFV.Mappings[0]
	second, err := NewMapping(cfg)
	require.NoError(t, err)

	descriptor := first.Descriptor()
	require.Equal(t, MappingSchemaVersion, descriptor.SchemaVersion)
	require.Equal(t, int64(105), descriptor.LocalFVI)
	require.Equal(t, int64(100), descriptor.RemoteFVI)
	require.Equal(t, 3, descriptor.LaneCount)
	require.Regexp(t, `^sha256:[0-9a-f]{64}$`, descriptor.Fingerprint)
	require.Equal(t, descriptor, second.Descriptor(), "input ordering must not affect mapping identity")
	require.NotContains(t, descriptor.Fingerprint, "105")

	require.Equal(t, MappingDescriptor{}, (*Mapping)(nil).Descriptor())
}

func TestValidateSuperset(t *testing.T) {
	t.Parallel()
	base := config.FailoverVersionTranslationConfig{
		FVI: config.IntMapping{Local: 105, Remote: 100},
		IFV: config.IFVTranslationConfig{Mappings: []config.IntMapping{
			{Local: 1, Remote: 1}, {Local: 2, Remote: 2},
		}},
	}

	additive := base
	additive.IFV.Mappings = append(append([]config.IntMapping(nil), base.IFV.Mappings...), config.IntMapping{Local: 105, Remote: 100})
	require.NoError(t, ValidateSuperset(base, additive))

	reordered := additive
	reordered.IFV.Mappings = append([]config.IntMapping(nil), additive.IFV.Mappings...)
	reordered.IFV.Mappings[0], reordered.IFV.Mappings[2] = reordered.IFV.Mappings[2], reordered.IFV.Mappings[0]
	require.NoError(t, ValidateSuperset(base, reordered))

	cases := []struct {
		name string
		next config.FailoverVersionTranslationConfig
		want string
	}{
		{name: "changed fvi", next: func() config.FailoverVersionTranslationConfig {
			next := additive
			next.FVI.Remote = 101
			return next
		}(), want: "fvi mapping cannot change"},
		{name: "removed pair", next: config.FailoverVersionTranslationConfig{
			FVI: base.FVI, IFV: config.IFVTranslationConfig{Mappings: []config.IntMapping{{Local: 1, Remote: 1}}},
		}, want: "removes"},
		{name: "changed pair", next: config.FailoverVersionTranslationConfig{
			FVI: base.FVI, IFV: config.IFVTranslationConfig{Mappings: []config.IntMapping{{Local: 1, Remote: 1}, {Local: 2, Remote: 3}}},
		}, want: "changes"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.ErrorContains(t, ValidateSuperset(base, tc.next), tc.want)
		})
	}
}

func TestTranslateVersionExactOverflowBoundary(t *testing.T) {
	t.Parallel()
	mapping, err := NewMapping(config.FailoverVersionTranslationConfig{
		FVI: config.IntMapping{Local: 1, Remote: 2},
		IFV: config.IFVTranslationConfig{Mappings: []config.IntMapping{{Local: 1, Remote: 2}}},
	})
	require.NoError(t, err)
	forward := mapping.LocalToRemote()
	maxEpoch := (int64(math.MaxInt64) - 2) / 2
	lastInput := maxEpoch + 1
	translated, err := forward.TranslateVersion(lastInput)
	require.NoError(t, err)
	require.Equal(t, maxEpoch*2+2, translated)
	_, err = forward.TranslateVersion(lastInput + 1)
	requireErrorCode(t, err, ErrorOverflow)
}

func FuzzTranslateVersionRoundTrip(f *testing.F) {
	mapping, err := NewMapping(config.FailoverVersionTranslationConfig{
		FVI: config.IntMapping{Local: 105, Remote: 100},
		IFV: config.IFVTranslationConfig{Mappings: []config.IntMapping{
			{Local: 1, Remote: 1}, {Local: 2, Remote: 2}, {Local: 105, Remote: 100},
		}},
	})
	require.NoError(f, err)
	for _, seed := range []int64{0, 1, 2, 105, 106, 107, math.MaxInt32} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, version int64) {
		if version < 0 {
			return
		}
		lane := int64(0)
		if version > 0 {
			lane = (version-1)%105 + 1
		}
		if lane != 0 && lane != 1 && lane != 2 && lane != 105 {
			return
		}
		translated, err := mapping.LocalToRemote().TranslateVersion(version)
		if err != nil {
			var translationErr *TranslationError
			require.ErrorAs(t, err, &translationErr)
			return
		}
		roundTrip, err := mapping.RemoteToLocal().TranslateVersion(translated)
		require.NoError(t, err)
		require.Equal(t, version, roundTrip)
	})
}

func BenchmarkTranslateVersion(b *testing.B) {
	forward := testMapping(b).LocalToRemote()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := forward.TranslateVersion(int64(i%1_000_000)*105 + 2); err != nil {
			b.Fatal(err)
		}
	}
}

func requireErrorCode(t *testing.T, err error, code ErrorCode) {
	t.Helper()
	var translationErr *TranslationError
	require.Error(t, err)
	require.True(t, errors.As(err, &translationErr))
	require.Equal(t, code, translationErr.Code)
}
