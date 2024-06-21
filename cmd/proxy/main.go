package main

import (
	"os"

	"github.com/temporalio/temporal-proxy/proxy"
)

func main() {
	if err := proxy.Run(os.Args); err != nil {
		panic(err)
	}
}
