package mux

import (
	"context"
	"fmt"

	"github.com/hashicorp/yamux"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/metrics"
	"github.com/temporalio/s2s-proxy/transport/grpcutil"
	"github.com/temporalio/s2s-proxy/transport/mux/session"
)

// NewGRPCMuxManager starts a MuxManager that will start a grpc.Server using the provided definition on every generated mux,
// and will manage the mux connections of the provided grpcutil.MultiClientConn such that they match the list of available muxes
// Caller usage: 1. Create the MultiClientConn, 2. call NewGRPCMuxManager, 3. MultiMuxManager.Start, 4. Use the grpcutil.MultiClientConn
// grpc.Server's will be started on every inbound mux automatically.
func NewGRPCMuxManager(ctx context.Context, cfg config.MuxTransportConfig, mcc *grpcutil.MultiClientConn, serverDefinition *grpc.Server, logger log.Logger) (MultiMuxManager, error) {
	// sane default for existing configs
	if cfg.NumConnections == 0 {
		cfg.NumConnections = 10
	}
	metricLabels := cfg.GetLabelValues()
	var muxProviderBuilder MuxProviderBuilder
	logger.Info(fmt.Sprintf("Applying %s mux provider from config: %v", string(cfg.Mode), cfg.Client))
	switch cfg.Mode {
	case config.ClientMode:
		muxProviderBuilder = func(cb AddNewMux, lifetime context.Context) (MuxProvider, error) {
			return NewMuxEstablisherProvider(lifetime, cfg.Name, cb, int64(cfg.NumConnections), cfg.Client, metricLabels, logger)
		}
	case config.ServerMode:
		muxProviderBuilder = func(cb AddNewMux, lifetime context.Context) (MuxProvider, error) {
			return NewMuxReceiverProvider(lifetime, cfg.Name, cb, int64(cfg.NumConnections), cfg.Server, metricLabels, logger)
		}
	default:
		return nil, fmt.Errorf("invalid multiplexed transport mode: name %s, mode %s", cfg.Name, cfg.Mode)
	}
	return NewCustomMultiMuxManager(ctx,
		cfg.Name,
		muxProviderBuilder,
		[]session.StartManagedComponentFn{
			registerYamuxObserverBuilder(cfg.Name, logger),
			registerGRPCServer(string(cfg.Mode), serverDefinition, metricLabels, logger),
		},
		[]OnConnectionListUpdate{mcc.OnConnectionListUpdate},
		logger,
	)
}

func registerGRPCServer(mode string, serverConfig *grpc.Server, metricLabels []string, logger log.Logger) session.StartManagedComponentFn {
	return func(lifetime context.Context, id string, session *yamux.Session) {
		go func() {
			logger.Info("Starting inbound server for mux", tag.NewStringTag("remote_addr", session.RemoteAddr().String()),
				tag.NewStringTag("mode", mode), tag.NewStringTag("mux_id", id))
			for lifetime.Err() == nil {
				_ = serverConfig.Serve(session)
				metrics.MuxServerDisconnected.WithLabelValues(metricLabels...).Inc()
			}
		}()
	}
}
