package joy2mac

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"tinygo.org/x/bluetooth"
)

const NINTENDO_SERVICE_UUID = "ab7de9be-89fe-49ad-828f-118f09df7fd0"
const INPUT_REPORT_CHARACTERISTIC_UUID = "ab7de9be-89fe-49ad-828f-118f09df7fd2"
const WRITE_COMMAND_CHARACTERISTIC_UUID = "649d4ac9-8eb7-4e6c-af44-1ea54fe5f005"

var ErrNintendoServiceNotFound = errors.New("nintendo service not found")
var ErrWriteCharacteristicNotFound = errors.New("write characteristic not found")
var ErrInputCharacteristicNotFound = errors.New("input characteristic not found")
var errCharacteristicNotFound = errors.New("characteristic not found")

type JoyconSession struct {
	address             bluetooth.Address
	device              bluetooth.Device
	nintendoService     bluetooth.DeviceService
	writeCharacteristic bluetooth.DeviceCharacteristic
	inputCharacteristic bluetooth.DeviceCharacteristic
	playerNo            int
	inputCh             chan<- InputData

	Connected bool
	Side      JoyconSide

	reConnecting bool
	mu           sync.Mutex
}

func CreateJoyconSession(
	candidate JoyconCandidate,
	playerNo int,
	inputCh chan<- InputData,
) *JoyconSession {
	return &JoyconSession{
		address:  candidate.Address,
		playerNo: playerNo,
		inputCh:  inputCh,
		Side:     candidate.Side,
	}
}

func (session *JoyconSession) Device() bluetooth.Device {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.device

}

func (session *JoyconSession) Address() bluetooth.Address {
	return session.address
}

func (session *JoyconSession) Disconnect() error {
	if session.Connected {
		return session.Device().Disconnect()
	}
	return nil
}

func (session *JoyconSession) attachDevice(device bluetooth.Device) {
	session.mu.Lock()
	session.device = device
	session.mu.Unlock()
}

func (session *JoyconSession) markConnected() {
	session.mu.Lock()
	defer session.mu.Unlock()

	session.Connected = true
	session.reConnecting = false
}

func (session *JoyconSession) markDisconnected() bool {
	session.mu.Lock()
	defer session.mu.Unlock()

	wasConnected := session.Connected
	session.Connected = false
	return wasConnected
}

func (session *JoyconSession) beginReconnect() bool {
	session.mu.Lock()
	defer session.mu.Unlock()

	if session.reConnecting {
		return false
	}

	session.reConnecting = true
	session.Connected = false
	return true
}

func (session *JoyconSession) endReconnect() {
	session.mu.Lock()
	defer session.mu.Unlock()

	session.reConnecting = false
}

func (session *JoyconSession) resetConnectionState() {
	session.mu.Lock()
	defer session.mu.Unlock()

	session.device = bluetooth.Device{}
	session.nintendoService = bluetooth.DeviceService{}
	session.writeCharacteristic = bluetooth.DeviceCharacteristic{}
	session.inputCharacteristic = bluetooth.DeviceCharacteristic{}
	session.Connected = false
}

func (session *JoyconSession) ReconnectLoop(manager *AdapterManager) {
	if !session.beginReconnect() {
		return
	}
	defer session.endReconnect()

	for {
		fmt.Printf("Attempting to reconnect to Joy-Con at %s...\n", session.address.String())
		err := manager.ConnectSession(session)

		if err == nil {
			fmt.Printf("Reconnected to Joy-Con at %s\n", session.address.String())
			return
		}

		fmt.Printf("Reconnect attempt failed for %s: %v\n", session.address.String(), err)
		time.Sleep(1 * time.Second)
	}
}

func (session *JoyconSession) setupConnection() error {
	if err := session.setupServices(); err != nil {
		return err
	}

	if err := session.setPlayerLEDs(session.playerNo); err != nil {
		return fmt.Errorf("failed to set player LEDs: %w", err)
	}
	time.Sleep(150 * time.Millisecond)

	if err := session.enableIMU(); err != nil {
		return fmt.Errorf("failed to enable IMU: %w", err)
	}

	return nil
}

func (session *JoyconSession) setupServices() error {
	nintendoServiceUUID, err := bluetooth.ParseUUID(NINTENDO_SERVICE_UUID)
	if err != nil {
		return err
	}

	services, err := session.discoverService(nintendoServiceUUID)
	if err != nil {
		return fmt.Errorf("failed to discover nintendo service: %w", err)
	}

	if len(services) == 0 {
		return ErrNintendoServiceNotFound
	}

	session.mu.Lock()
	session.nintendoService = services[0]
	session.mu.Unlock()

	writeUUID, err := bluetooth.ParseUUID(WRITE_COMMAND_CHARACTERISTIC_UUID)
	if err != nil {
		return fmt.Errorf("failed to parse write characteristic UUID: %w", err)
	}

	inputUUID, err := bluetooth.ParseUUID(INPUT_REPORT_CHARACTERISTIC_UUID)
	if err != nil {
		return fmt.Errorf("failed to parse input characteristic UUID: %w", err)
	}

	writeChar, err := session.discoverCharacteristic(writeUUID)
	if err != nil {
		if errors.Is(err, errCharacteristicNotFound) {
			return ErrWriteCharacteristicNotFound
		}
		return fmt.Errorf("failed to discover write characteristic: %w", err)
	}

	inputChar, err := session.discoverCharacteristic(inputUUID)
	if err != nil {
		if errors.Is(err, errCharacteristicNotFound) {
			return ErrInputCharacteristicNotFound
		}
		return fmt.Errorf("failed to discover input characteristic: %w", err)
	}

	session.mu.Lock()
	session.writeCharacteristic = writeChar
	session.inputCharacteristic = inputChar
	session.mu.Unlock()

	return nil
}

func (session *JoyconSession) discoverService(serviceUUID bluetooth.UUID) ([]bluetooth.DeviceService, error) {
	device := session.Device()
	services, err := device.DiscoverServices([]bluetooth.UUID{serviceUUID})

	if err != nil {
		return nil, fmt.Errorf("service discovery error: %w", err)
	}
	return services, nil
}

func (session *JoyconSession) discoverCharacteristic(characteristicUUID bluetooth.UUID) (bluetooth.DeviceCharacteristic, error) {
	session.mu.Lock()
	nintendoService := session.nintendoService
	session.mu.Unlock()

	characteristics, err := nintendoService.DiscoverCharacteristics([]bluetooth.UUID{characteristicUUID})
	if err != nil {
		return bluetooth.DeviceCharacteristic{}, fmt.Errorf("characteristic discovery error: %w", err)
	}
	if len(characteristics) == 0 {
		return bluetooth.DeviceCharacteristic{}, errCharacteristicNotFound
	}

	return characteristics[0], nil
}

func (session *JoyconSession) writeCommand(
	commandID byte,
	subCommandID byte,
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

	return session.writeCharacteristic.WriteWithoutResponse(buffer)
}

func (session *JoyconSession) setPlayerLEDs(playerNo int) error {
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

	_, err := session.writeCommand(LED_COMMAND_PREFIX, SET_LED_COMMAND, []byte{LED_PATTERN_ID[playerNo]})
	return err
}

func (session *JoyconSession) enableIMU() error {
	ENABLE_IMU_1 := []byte{0x0c, 0x91, 0x01, 0x02, 0x00, 0x04, 0x00, 0x00, 0xFF, 0x00, 0x00, 0x00}
	ENABLE_IMU_2 := []byte{0x0c, 0x91, 0x01, 0x04, 0x00, 0x04, 0x00, 0x00, 0xFF, 0x00, 0x00, 0x00}

	_, err := session.writeCharacteristic.WriteWithoutResponse(ENABLE_IMU_1)
	if err != nil {
		return err
	}
	time.Sleep(500 * time.Millisecond)

	_, err = session.writeCharacteristic.WriteWithoutResponse(ENABLE_IMU_2)
	if err != nil {
		return err
	}

	return nil
}

func (session *JoyconSession) StartInputNotification(outCh chan<- InputData) error {
	return session.inputCharacteristic.EnableNotifications(
		func(buf []byte) {
			if len(buf) == 0 {
				return
			}
			payload := append([]byte(nil), buf...)
			select {
			case outCh <- InputData{
				PlayerNo: session.playerNo,
				Side:     session.Side,
				Data:     payload,
			}:
			default:
			}
		})
}
