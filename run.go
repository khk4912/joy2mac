package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	joy2mac "github.com/khk4912/joy2mac/src"
	"tinygo.org/x/bluetooth"
)

func main() {
	candidates, err := joy2mac.ScanJoycons()
	devices := make([]bluetooth.Device, 0, len(candidates))

	if err != nil {
		fmt.Printf("Error scanning for Joy-Con devices: %v\n", err)
		return
	}

	if len(candidates) == 0 {
		println("No Joy-Con candidates found. Exiting.")
		return
	}

	for i, candidate := range candidates {
		playerNo := i + 1
		if d, err := joy2mac.StartJoyconConnection(candidate, playerNo); err != nil {
			fmt.Printf("Connection failed for %s: %v\n", candidate.AddressString, err)
		} else {
			devices = append(devices, d)
		}
	}

	if len(devices) == 0 {
		fmt.Println("No Joy-Con connected. Exiting.")
		return
	}

	fmt.Println("Connected. Listening for input reports. Press Ctrl+C to exit.")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM, syscall.SIGKILL)

	<-sigCh

	fmt.Println("\nShutting down...")
	for _, d := range devices {
		_ = d.Disconnect()
	}
}
