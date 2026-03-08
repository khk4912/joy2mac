package main

import (
	"fmt"
	"os"

	joy2mac "github.com/khk4912/joy2mac/src"
)

func main() {
	mode := "single"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}

	switch mode {
	case "single":
		joy2mac.StartSingleJoyconMode()
	case "dual":
		joy2mac.StartDualJoyconMode()
	default:
		fmt.Fprintf(os.Stderr, "usage: %s [single|dual]\n", os.Args[0])
		os.Exit(2)
	}
}
