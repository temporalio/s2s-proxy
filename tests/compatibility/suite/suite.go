//go:build testcompatibility

package suite

import (
	"github.com/stretchr/testify/suite"

	"github.com/temporalio/s2s-proxy/tests/compatibility/topology"
)

// BaseSuite is the shared base for all compatibility suites, providing access to the topology and topology operations.
type BaseSuite struct {
	suite.Suite
	top topology.Topology
}

func (s *BaseSuite) Ops() Ops {
	return Ops{t: s.T(), top: s.top}
}
