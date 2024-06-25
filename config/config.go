// The MIT License

// Copyright (c) 2020 Temporal Technologies Inc.  All rights reserved.

// Copyright (c) 2020 Uber Technologies, Inc.

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package config

type (
	// RPC contains the rpc config items
	RPC struct {
		// GRPCPort is the port on which gRPC will listen
		GRPCPort int `yaml:"grpcPort"`
		// Port used for membership listener
		MembershipPort int `yaml:"membershipPort"`
		// BindOnLocalHost is true if localhost is the bind address
		// if neither BindOnLocalHost nor BindOnIP are set then an
		// an attempt to discover an address is made
		BindOnLocalHost bool `yaml:"bindOnLocalHost"`
		// BindOnIP can be used to bind service on specific ip (eg. `0.0.0.0` or `::`)
		// check net.ParseIP for supported syntax
		// mutually exclusive with `BindOnLocalHost` option
		BindOnIP string `yaml:"bindOnIP"`
		// HTTPPort is the port on which HTTP will listen. If unset/0, HTTP will be
		// disabled. This setting only applies to the frontend service.
		HTTPPort int `yaml:"httpPort"`
		// HTTPAdditionalForwardedHeaders adds additional headers to the default set
		// forwarded from HTTP to gRPC. Any value with a trailing * will match the prefix before
		// the asterisk (eg. `x-internal-*`)
		HTTPAdditionalForwardedHeaders []string `yaml:"httpAdditionalForwardedHeaders"`
	}
)
