package main

import (
	"os"

	"github.com/Basic-Components/thompsonsampling-orderrpc/thompsonsampling"
)

func main() {
	thompsonsampling.ServNode.Parse(os.Args)
}
