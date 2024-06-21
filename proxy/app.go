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

package proxy

import (
	"github.com/urfave/cli/v2"
)

const (
	RemoteAddressFlag = "remote-address"
	ProxyVersion      = "0.0.1"
)

// Run executes the saas-agent
func Run(args []string) error {
	app := buildCLIOptions()
	return app.Run(args)
}

func buildCLIOptions() *cli.App {
	app := cli.NewApp()
	app.Name = "tempora-proxy"
	app.Usage = "Temporal proxy"
	app.Version = ProxyVersion

	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:    RemoteAddressFlag,
			Aliases: []string{"ra"},
		},
	}

	app.Commands = []*cli.Command{
		{
			Name:   "start",
			Usage:  "Starts the worker. Will block indefinitely",
			Action: startProxy,
		},
	}

	return app
}

func startProxy(c *cli.Context) error {
	return nil
}
