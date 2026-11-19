package failoverversion

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/temporalio/s2s-proxy/config"
)

const MappingSchemaVersion = "v1"

// MappingDescriptor is a safe, stable description of a compiled mapping. It
// deliberately omits individual lane pairs: the fingerprint is sufficient to
// compare replicas and retained migration evidence without disclosing the
// complete configuration through debug surfaces.
type MappingDescriptor struct {
	SchemaVersion string `json:"schema_version"`
	LocalFVI      int64  `json:"local_fvi"`
	RemoteFVI     int64  `json:"remote_fvi"`
	LaneCount     int    `json:"lane_count"`
	Fingerprint   string `json:"fingerprint"`
}

type Mapping struct {
	localFVI      int64
	remoteFVI     int64
	localToRemote map[int64]int64
	remoteToLocal map[int64]int64
	descriptor    MappingDescriptor
}

type Translator struct {
	direction  Direction
	inputFVI   int64
	outputFVI  int64
	lanes      map[int64]int64
	descriptor MappingDescriptor
}

func NewMapping(cfg config.FailoverVersionTranslationConfig) (*Mapping, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	forward := make(map[int64]int64, len(cfg.IFV.Mappings))
	inverse := make(map[int64]int64, len(cfg.IFV.Mappings))
	for _, pair := range cfg.IFV.Mappings {
		forward[pair.Local] = pair.Remote
		inverse[pair.Remote] = pair.Local
	}
	return &Mapping{
		localFVI:      cfg.FVI.Local,
		remoteFVI:     cfg.FVI.Remote,
		localToRemote: forward,
		remoteToLocal: inverse,
		descriptor:    describeConfig(cfg),
	}, nil
}

// Descriptor returns a copy of the immutable mapping description.
func (m *Mapping) Descriptor() MappingDescriptor {
	if m == nil {
		return MappingDescriptor{}
	}
	return m.descriptor
}

// ValidateSuperset validates a safe mapping update. Once translated data has
// been persisted, FVIs and existing lane identities are immutable; an update
// may only add new order-preserving lane pairs.
func ValidateSuperset(previous, next config.FailoverVersionTranslationConfig) error {
	if err := previous.Validate(); err != nil {
		return fmt.Errorf("previous mapping: %w", err)
	}
	if err := next.Validate(); err != nil {
		return fmt.Errorf("next mapping: %w", err)
	}
	if previous.FVI != next.FVI {
		return fmt.Errorf("fvi mapping cannot change after activation")
	}
	nextPairs := make(map[int64]int64, len(next.IFV.Mappings))
	for _, pair := range next.IFV.Mappings {
		nextPairs[pair.Local] = pair.Remote
	}
	for index, pair := range previous.IFV.Mappings {
		remote, ok := nextPairs[pair.Local]
		if !ok {
			return fmt.Errorf("next mapping removes ifv.mappings[%d]", index)
		}
		if remote != pair.Remote {
			return fmt.Errorf("next mapping changes ifv.mappings[%d]", index)
		}
	}
	return nil
}

func describeConfig(cfg config.FailoverVersionTranslationConfig) MappingDescriptor {
	pairs := append([]config.IntMapping(nil), cfg.IFV.Mappings...)
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].Local == pairs[j].Local {
			return pairs[i].Remote < pairs[j].Remote
		}
		return pairs[i].Local < pairs[j].Local
	})
	var canonical strings.Builder
	fmt.Fprintf(&canonical, "%s|%d|%d", MappingSchemaVersion, cfg.FVI.Local, cfg.FVI.Remote)
	for _, pair := range pairs {
		fmt.Fprintf(&canonical, "|%d:%d", pair.Local, pair.Remote)
	}
	sum := sha256.Sum256([]byte(canonical.String()))
	return MappingDescriptor{
		SchemaVersion: MappingSchemaVersion,
		LocalFVI:      cfg.FVI.Local,
		RemoteFVI:     cfg.FVI.Remote,
		LaneCount:     len(pairs),
		Fingerprint:   "sha256:" + hex.EncodeToString(sum[:]),
	}
}

func (m *Mapping) LocalToRemote() Translator {
	return Translator{direction: LocalToRemote, inputFVI: m.localFVI, outputFVI: m.remoteFVI, lanes: m.localToRemote, descriptor: m.descriptor}
}

func (m *Mapping) RemoteToLocal() Translator {
	return Translator{direction: RemoteToLocal, inputFVI: m.remoteFVI, outputFVI: m.localFVI, lanes: m.remoteToLocal, descriptor: m.descriptor}
}

func (t Translator) Direction() Direction                 { return t.direction }
func (t Translator) MappingDescriptor() MappingDescriptor { return t.descriptor }

func (t Translator) TranslateVersion(version int64) (int64, error) {
	if version == 0 {
		return 0, nil
	}
	if version < 0 {
		return 0, NewTranslationError(ErrorUnknownLane, t.direction, "", "", unexpectedValueCause("version"))
	}
	epoch := (version - 1) / t.inputFVI
	lane := ((version - 1) % t.inputFVI) + 1
	translatedLane, ok := t.lanes[lane]
	if !ok {
		return 0, NewTranslationError(ErrorUnknownLane, t.direction, "", "", unexpectedValueCause("lane"))
	}
	if epoch > (math.MaxInt64-translatedLane)/t.outputFVI {
		return 0, NewTranslationError(ErrorOverflow, t.direction, "", "", unexpectedValueCause("version"))
	}
	return epoch*t.outputFVI + translatedLane, nil
}

func (t Translator) TranslateFailoverVersionIncrement(fvi int64) (int64, error) {
	if fvi != t.inputFVI {
		return 0, NewTranslationError(ErrorUnexpectedFVI, t.direction, "", "", unexpectedValueCause("failover version increment"))
	}
	return t.outputFVI, nil
}

func (t Translator) TranslateLane(lane int64) (int64, error) {
	if lane == 0 {
		return 0, nil
	}
	if lane < 0 {
		return 0, NewTranslationError(ErrorUnknownLane, t.direction, "", "", unexpectedValueCause("lane"))
	}
	translated, ok := t.lanes[lane]
	if !ok {
		return 0, NewTranslationError(ErrorUnknownLane, t.direction, "", "", unexpectedValueCause("lane"))
	}
	return translated, nil
}

func (t Translator) TranslateLaneInt32(lane int32) (int32, error) {
	translated, err := t.TranslateLane(int64(lane))
	if err != nil {
		return 0, err
	}
	if translated > math.MaxInt32 {
		return 0, NewTranslationError(ErrorOverflow, t.direction, "", "", unexpectedValueCause("cluster ID"))
	}
	return int32(translated), nil
}
