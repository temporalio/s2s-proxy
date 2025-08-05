package transport

import (
	"encoding/json"
	"fmt"
	"log"
	"sync/atomic"

	_ "google.golang.org/grpc" // to register pick_first
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/endpointsharding"
	"google.golang.org/grpc/balancer/pickfirst/pickfirstleaf"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/serviceconfig"
)

func init() {
	log.Printf("register balancer")
	balancer.Register(customRoundRobinBuilder{})
}

const customRRName = "custom_round_robin"

type customRRConfig struct {
	serviceconfig.LoadBalancingConfig `json:"-"`
}

type customRoundRobinBuilder struct{}

func (customRoundRobinBuilder) ParseConfig(s json.RawMessage) (out serviceconfig.LoadBalancingConfig, err error) {
	log.Printf("lb ParseConfig")
	defer func() {
		log.Printf("lb ParseConfig >> %v, %v", out, err)

	}()
	lbConfig := &customRRConfig{}

	if err := json.Unmarshal(s, lbConfig); err != nil {
		return nil, fmt.Errorf("custom-round-robin: unable to unmarshal customRRConfig: %v", err)
	}
	return lbConfig, nil
}

func (customRoundRobinBuilder) Name() string {
	return customRRName
}

func (customRoundRobinBuilder) Build(cc balancer.ClientConn, bOpts balancer.BuildOptions) balancer.Balancer {
	log.Printf("lb Build: %v, %v", cc, bOpts)
	crr := &customRoundRobin{
		ClientConn: cc,
		bOpts:      bOpts,
	}
	crr.Balancer = endpointsharding.NewBalancer(crr, bOpts, balancer.Get(pickfirstleaf.Name).Build, endpointsharding.Options{})
	return crr
}

type customRoundRobin struct {
	// All state and operations on this balancer are either initialized at build
	// time and read only after, or are only accessed as part of its
	// balancer.Balancer API (UpdateState from children only comes in from
	// balancer.Balancer calls as well, and children are called one at a time),
	// in which calls are guaranteed to come synchronously. Thus, no extra
	// synchronization is required in this balancer.
	balancer.Balancer
	balancer.ClientConn
	bOpts balancer.BuildOptions

	cfg atomic.Pointer[customRRConfig]
}

func (crr *customRoundRobin) UpdateClientConnState(state balancer.ClientConnState) (err error) {
	log.Printf("lb UpdateClienConnState: %#v", state)
	defer func() {
		log.Printf("lb UpdateClienConnState: %v", err)
	}()
	crrCfg, ok := state.BalancerConfig.(*customRRConfig)
	if !ok {
		return balancer.ErrBadResolverState
	}

	// Optional process store something here based on number of endpoints.

	if el := state.ResolverState.Endpoints; len(el) != 2 {
		return fmt.Errorf("UpdateClientConnState wants two endpoints, got: %v", el)
	}
	crr.cfg.Store(crrCfg)
	// A call to UpdateClientConnState should always produce a new Picker.  That
	// is guaranteed to happen since the aggregator will always call
	// UpdateChildState in its UpdateClientConnState.
	return crr.Balancer.UpdateClientConnState(balancer.ClientConnState{
		ResolverState: state.ResolverState,
	})
}

func (crr *customRoundRobin) UpdateState(state balancer.State) {
	log.Printf("lb UpdateState: %#v", state)

	if state.ConnectivityState != connectivity.Ready {
		// Delegate to default behavior/picker from below.
		crr.ClientConn.UpdateState(state)
		return
	}

	// collect the ready pickers
	childStates := endpointsharding.ChildStatesFromPicker(state.Picker)
	var readyPickers []balancer.Picker
	for _, childState := range childStates {
		if childState.State.ConnectivityState == connectivity.Ready {
			readyPickers = append(readyPickers, childState.State.Picker)
		}
	}

	if len(readyPickers) < 2 {
		log.Printf("lb UpdateState -- %d ready pickers (waiting for 2)", len(readyPickers))
		//crr.ClientConn.UpdateState(state)
		return
	}

	log.Printf("lb UpdateState -- %d ready pickers", len(readyPickers))

	picker := &customRoundRobinPicker{
		pickers: readyPickers,
	}
	crr.ClientConn.UpdateState(balancer.State{
		ConnectivityState: connectivity.Ready,
		Picker:            picker,
	})
}

type customRoundRobinPicker struct {
	pickers []balancer.Picker
	//chooseSecond uint32
	next uint32
}

func (crrp *customRoundRobinPicker) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	next := int(atomic.AddUint32(&crrp.next, 1))

	log.Printf("lb Pick: %d %v", next, info)
	childPicker := crrp.pickers[next%len(crrp.pickers)]
	return childPicker.Pick(info)
}
