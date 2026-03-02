package joy2mac

import (
	"fmt"
	"time"

	"tinygo.org/x/bluetooth"
)

const NINTENDO_SERVICE_UUID = "ab7de9be-89fe-49ad-828f-118f09df7fd0"
const INPUT_REPORT_CHARACTERISTIC_UUID = "ab7de9be-89fe-49ad-828f-118f09df7fd2"
const WRITE_COMMAND_CHARACTERISTIC_UUID = "649d4ac9-8eb7-4e6c-af44-1ea54fe5f005"

func StartJoyconConnection(candidate JoyconCandidate, playerNo int) (bluetooth.Device, error) {
	adapter := bluetooth.DefaultAdapter

	const maxAttempts = 3
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		fmt.Printf("\nAttempting to connect to device at %s (attempt %d/%d)...\n", candidate.AddressString, attempt, maxAttempts)

		device, err := adapter.Connect(candidate.Address, bluetooth.ConnectionParams{
			ConnectionTimeout: bluetooth.NewDuration(20 * time.Second),
		})
		if err != nil {
			lastErr = err
			fmt.Printf("Connect attempt failed: %v\n", err)
			time.Sleep(2 * time.Second)
			continue
		}

		if err := onConnected(device, playerNo); err != nil {
			_ = device.Disconnect()
			return bluetooth.Device{}, fmt.Errorf("connected but setup failed: %w", err)
		}

		return device, nil
	}

	return bluetooth.Device{}, fmt.Errorf("failed to connect after %d attempts: %w", maxAttempts, lastErr)
}

func discoverService(device bluetooth.Device, serviceUUID bluetooth.UUID) ([]bluetooth.DeviceService, error) {
	services, err := device.DiscoverServices([]bluetooth.UUID{serviceUUID})
	if err != nil {
		return nil, fmt.Errorf("service discovery error: %w", err)
	}

	return services, nil
}

func discoverCharacteristic(service bluetooth.DeviceService, characteristicUUID bluetooth.UUID) ([]bluetooth.DeviceCharacteristic, error) {
	characteristics, err := service.DiscoverCharacteristics([]bluetooth.UUID{characteristicUUID})
	if err != nil {
		return nil, fmt.Errorf("characteristic discovery error: %w", err)
	}

	return characteristics, nil
}

func onConnected(device bluetooth.Device, playerNo int) error {
	fmt.Printf("Connected to device: %s\n", device.Address.UUID)

	nintendoServiceUUID, err := bluetooth.ParseUUID(NINTENDO_SERVICE_UUID)
	if err != nil {
		return fmt.Errorf("failed to parse nintendo service UUID: %w", err)
	}

	writeUUID, err := bluetooth.ParseUUID(WRITE_COMMAND_CHARACTERISTIC_UUID)
	if err != nil {
		return fmt.Errorf("failed to parse write characteristic UUID: %w", err)
	}
	inputUUID, err := bluetooth.ParseUUID(INPUT_REPORT_CHARACTERISTIC_UUID)
	if err != nil {
		return fmt.Errorf("failed to parse input characteristic UUID: %w", err)
	}

	services, err := discoverService(device, nintendoServiceUUID)
	if err != nil || len(services) == 0 {
		return fmt.Errorf("Failed to discover nintendo service!")
	}

	fmt.Printf("Services discovered: %d\n", len(services))
	characteristics, err := services[0].DiscoverCharacteristics(nil)

	if err != nil {
		return fmt.Errorf("Failed to discover nintendo service characteristics: %w", err)
	}

	fmt.Println("Nintendo service found!")
	for _, c := range characteristics {
		fmt.Printf("  Characteristic: %s\n", c.UUID())
	}

	nintendoService := services[0]
	writeCharacteristics, err := discoverCharacteristic(nintendoService, writeUUID)
	if err != nil || len(writeCharacteristics) == 0 {
		return fmt.Errorf("Failed to discover write characteristic!")
	}

	inputCharacteristics, err := discoverCharacteristic(nintendoService, inputUUID)
	if err != nil || len(inputCharacteristics) == 0 {
		return fmt.Errorf("Failed to discover input characteristic!")
	}

	fmt.Println("\nSetting player LEDs...")
	err = setPlayerLEDs(writeCharacteristics[0], playerNo)

	if err != nil {
		return fmt.Errorf("setPlayerLEDs failed: %w\n", err)
	}
	time.Sleep(150 * time.Millisecond)

	fmt.Println("Enabling IMU...")
	err = enable_imu(writeCharacteristics[0])
	if err != nil {
		return fmt.Errorf("enable_imu failed: %w\n", err)
	}

	fmt.Println("Enabling input notifications...")
	err = inputCharacteristics[0].EnableNotifications(func(buf []byte) {
		if len(buf) == 0 {
			return
		}
		fmt.Printf("Input report (%d): % X\n", len(buf), buf)
	})
	if err != nil {
		return fmt.Errorf("enable notifications failed: %w", err)
	}

	fmt.Println("Joy-Con notification stream is active.")
	return nil
}

func writeCommand(
	writeCharacteristic bluetooth.DeviceCharacteristic,
	commandID byte, subCommandID byte,
	cmd []byte) (int, error) {

	payload := make([]byte, len(cmd))
	copy(payload, cmd)

	if len(payload) < 8 {
		padding := make([]byte, 8-len(payload))
		payload = append(payload, padding...)
	}

	buffer := []byte{
		commandID,
		0x91,
		0x01,
		subCommandID,
		0x00,
		byte(len(payload)),
		0x00,
		0x00,
	}
	buffer = append(buffer, payload...)

	return writeCharacteristic.WriteWithoutResponse(buffer)
}

func setPlayerLEDs(writeCharacteristic bluetooth.DeviceCharacteristic, playerNo int) error {
	playerNo = max(1, min(playerNo, 8))

	LED_PATTERN_ID := map[int]byte{
		1: 0x01,
		2: 0x03,
		3: 0x07,
		4: 0x0F,
		5: 0x09,
		6: 0x05,
		7: 0x0D,
		8: 0x06,
	}

	// Send the LED command
	var LED_COMMAND_PREFIX byte = 0x09
	var SET_LED_COMMAND byte = 0x07

	_, err := writeCommand(writeCharacteristic, LED_COMMAND_PREFIX, SET_LED_COMMAND, []byte{LED_PATTERN_ID[playerNo]})
	if err != nil {
		return fmt.Errorf("failed to set player LEDs: %w", err)
	}

	return err
}

func enable_imu(writeCharacteristic bluetooth.DeviceCharacteristic) error {
	ENABLE_IMU_1 := []byte{0x0c, 0x91, 0x01, 0x02, 0x00, 0x04, 0x00, 0x00, 0x2f, 0x00, 0x00, 0x00}
	ENABLE_IMU_2 := []byte{0x0c, 0x91, 0x01, 0x04, 0x00, 0x04, 0x00, 0x00, 0x2f, 0x00, 0x00, 0x00}

	writeCharacteristic.WriteWithoutResponse(ENABLE_IMU_1)
	time.Sleep(500 * time.Millisecond)
	writeCharacteristic.WriteWithoutResponse(ENABLE_IMU_2)

	return nil
}
