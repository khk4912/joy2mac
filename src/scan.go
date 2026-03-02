package joy2mac

import (
	"fmt"
	"time"

	"tinygo.org/x/bluetooth"
)

const JOYCON_MANUFACTURER_ID = 0x553

var JOYCON_MANUFACTURER_PREFIX = []byte{1, 0, 3, 126}

const (
	maxFoundJoycons = 2
	scanTimeout     = 10 * time.Second
)

type JoyconCandidate struct {
	Address       bluetooth.Address
	AddressString string
}

func ScanJoycons() ([]JoyconCandidate, error) {
	adapter := bluetooth.DefaultAdapter

	err := adapter.Enable()
	if err != nil {
		return nil, err
	}

	candidates := make([]JoyconCandidate, 0, maxFoundJoycons)

	timer := time.AfterFunc(scanTimeout, func() {
		if err := stopScan(adapter, fmt.Sprintf("timeout (%s)", scanTimeout)); err != nil {
			fmt.Printf("Error stopping scan on timeout: %v\n", err)
		}
	})
	defer timer.Stop()

	fmt.Printf("Scanning for Joy-Con 2 (max %d, timeout %s)...\n\n", maxFoundJoycons, scanTimeout)

	err = adapter.Scan(
		func(a *bluetooth.Adapter, result bluetooth.ScanResult) {
			err := onAdapterScan(a, result, &candidates)

			if err != nil {
				fmt.Printf("Error during scan callback: %v\n", err)
			}
		})

	if err != nil {
		return nil, fmt.Errorf("Failed to scan for Joy-Con devices: %w", err)
	}

	fmt.Printf("Scan complete. Joy-Con candidates found: %d\n", len(candidates))
	return candidates, nil
}

func stopScan(a *bluetooth.Adapter, reason string) error {
	fmt.Printf("Stopping scan: %s\n", reason)

	if stopErr := a.StopScan(); stopErr != nil {
		return fmt.Errorf("Failed to stop scan: %w", stopErr)
	}

	return nil
}

func onAdapterScan(
	a *bluetooth.Adapter,
	result bluetooth.ScanResult,
	candidates *[]JoyconCandidate) error {

	seen := map[string]struct{}{}
	found := 0

	manufactureData := result.ManufacturerData()

	if len(manufactureData) == 0 {
		return nil
	}

	deviceInfo := manufactureData[0]
	if deviceInfo.CompanyID != JOYCON_MANUFACTURER_ID {
		return nil
	}

	if len(deviceInfo.Data) < len(JOYCON_MANUFACTURER_PREFIX) {
		return nil
	}

	for i, b := range JOYCON_MANUFACTURER_PREFIX {
		if deviceInfo.Data[i] != b {
			return nil
		}
	}

	addr := result.Address.String()

	if _, exists := seen[addr]; exists {
		return nil
	}

	seen[addr] = struct{}{}

	found++
	count := found

	*candidates = append(*candidates, JoyconCandidate{
		Address:       result.Address,
		AddressString: addr,
	})

	fmt.Printf("Possible Joy-Con 2 found #%d\n", count)
	fmt.Printf("  Address: %s\n", addr)

	if count >= maxFoundJoycons {
		if err := stopScan(a, fmt.Sprintf("found %d Joy-Con device(s)", maxFoundJoycons)); err != nil {
			return fmt.Errorf("Error stopping scan: %w", err)
		}
	}

	return nil
}
