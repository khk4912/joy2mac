package joy2mac

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func StartSingleJoyconMode() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	adapterManger := NewAdapterManager(1)
	candidates, err := adapterManger.ScanJoycons()

	if err != nil {
		fmt.Printf("Failed to scan Joy-Con devices: %v\n", err)
		adapterManger.Shutdown()
		return
	}

	if len(candidates) != 1 {
		fmt.Println("Expected 1 Joy-Con device, found", len(candidates))
		fmt.Println("Stopping...")

		adapterManger.Shutdown()
		return
	}

	inputCh := make(chan InputData, 1)
	session := CreateJoyconSession(candidates[0], 1, inputCh)
	adapterManger.AddJoyconSession(session)

	err = adapterManger.ConnectSession(session)
	if err != nil {
		fmt.Printf("Failed to connect to Joy-Con at %s: %v\n", candidates[0].AddressString, err)
		adapterManger.Shutdown()
		return
	}

	SingleJoyconHandler(ctx, inputCh)

	fmt.Println("\nShutting down...")
	adapterManger.Shutdown()

}

func StartDualJoyconMode() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	adapterManger := NewAdapterManager(2)
	candidates, err := adapterManger.ScanJoycons()

	if err != nil {
		fmt.Printf("Failed to scan Joy-Con devices: %v\n", err)
		adapterManger.Shutdown()
		return
	}

	if len(candidates) != 2 {
		fmt.Println("Expected 2 Joy-Con devices, found", len(candidates))
		fmt.Println("Stopping...")

		adapterManger.Shutdown()
		return
	}

	leftInputCh := make(chan InputData, 1)
	rightInputCh := make(chan InputData, 1)

	session1 := CreateJoyconSession(candidates[0], 1, leftInputCh)
	session2 := CreateJoyconSession(candidates[1], 2, rightInputCh)

	adapterManger.AddJoyconSession(session1)
	adapterManger.AddJoyconSession(session2)

	err = adapterManger.ConnectSession(session1)
	if err != nil {
		fmt.Printf("Failed to connect to Joy-Con at %s: %v\n", candidates[0].AddressString, err)
		adapterManger.Shutdown()
		return
	}

	err = adapterManger.ConnectSession(session2)
	if err != nil {
		fmt.Printf("Failed to connect to Joy-Con at %s: %v\n", candidates[1].AddressString, err)
		adapterManger.Shutdown()
		return
	}

	DualJoyconHandler(ctx, leftInputCh, rightInputCh)

	fmt.Println("\nShutting down...")
	adapterManger.Shutdown()

}
