package common

import (
	"go.temporal.io/server/common/log/tag"
)

func ServiceTag(sv string) tag.ZapTag {
	return tag.NewStringTag("service", sv)
}
