package outboundauth

import (
	"google.golang.org/grpc/credentials"
)

type Destination string

const (
	DestinationLocal  Destination = "local"
	DestinationRemote Destination = "remote"
)

type Target struct {
	ClusterConnection string
	Destination       Destination
	Address           string
	Transport         string
}

type BuildRequest struct {
	Target     Target
	Properties map[string]string
}

type BuiltCredentials struct {
	PerRPC            credentials.PerRPCCredentials
	OwnedMetadataKeys []string
}

type Factory interface {
	Build(BuildRequest) (BuiltCredentials, error)
}

type FactoryFunc func(BuildRequest) (BuiltCredentials, error)

func (f FactoryFunc) Build(req BuildRequest) (BuiltCredentials, error) {
	return f(req)
}

type Registration struct {
	Name    string
	Factory Factory
}
