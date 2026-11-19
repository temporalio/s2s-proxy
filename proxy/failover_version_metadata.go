package proxy

import (
	"errors"
	"strconv"

	"go.temporal.io/server/client/history"
	"google.golang.org/grpc/metadata"

	"github.com/temporalio/s2s-proxy/failoverversion"
)

type clusterIDTranslator interface {
	TranslateLaneInt32(int32) (int32, error)
}

type directionalClusterIDTranslator interface {
	Direction() failoverversion.Direction
}

func translateClusterShardMetadata(md metadata.MD, translator clusterIDTranslator) (metadata.MD, error) {
	if translator == nil {
		return md.Copy(), nil
	}
	translated := md.Copy()
	direction := failoverversion.Direction("")
	if directional, ok := translator.(directionalClusterIDTranslator); ok {
		direction = directional.Direction()
	}
	for _, key := range []string{history.MetadataKeyClientClusterID, history.MetadataKeyServerClusterID} {
		values := md.Get(key)
		if len(values) != 1 || values[0] == "" {
			return nil, failoverversion.NewTranslationError(
				failoverversion.ErrorInvalidClusterID,
				direction,
				"grpc.metadata",
				key,
				errors.New("cluster ID metadata must contain exactly one value"),
			)
		}
		for _, r := range values[0] {
			if r < '0' || r > '9' {
				return nil, failoverversion.NewTranslationError(
					failoverversion.ErrorInvalidClusterID,
					direction,
					"grpc.metadata",
					key,
					errors.New("cluster ID metadata is not unsigned decimal"),
				)
			}
		}
		clusterID, err := strconv.ParseInt(values[0], 10, 32)
		if err != nil || clusterID <= 0 {
			return nil, failoverversion.NewTranslationError(
				failoverversion.ErrorInvalidClusterID,
				direction,
				"grpc.metadata",
				key,
				errors.New("cluster ID metadata is out of range"),
			)
		}
		mapped, err := translator.TranslateLaneInt32(int32(clusterID))
		if err != nil {
			return nil, failoverversion.WithField(err, "grpc.metadata", key)
		}
		if mapped <= 0 {
			return nil, failoverversion.NewTranslationError(
				failoverversion.ErrorInvalidClusterID,
				direction,
				"grpc.metadata",
				key,
				errors.New("translated cluster ID is not positive"),
			)
		}
		translated.Set(key, strconv.FormatInt(int64(mapped), 10))
	}
	return translated, nil
}
