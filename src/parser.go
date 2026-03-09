package joy2mac

import "encoding/binary"

func parseInputData(joyconInputch <-chan InputData) <-chan JoyconState {
	parsedCh := make(chan JoyconState, 1)

	go func() {
		defer close(parsedCh)

		for input := range joyconInputch {
			raw := append([]byte(nil), input.Data...)
			state := JoyconState{
				PlayerNo:    input.PlayerNo,
				Side:        input.Side,
				Raw:         raw,
				Temperature: parseTemperature(raw),
				// Accel:       parseAccel(input.Data),
				// Gyro:        parseGyro(input.Data),
				Voltage: parseVoltage(input.Data),
			}

			// if state.Side == LeftSide {
			// 	state.Stick = parseLeftStick(input.Data)
			// 	state.Buttons = parseLeftButtons(input.Data)
			// } else if state.Side == RightSide {
			// 	state.Buttons = parseRightButtons(input.Data)
			// 	state.Stick = parseRightStick(input.Data)
			// }

			parsedCh <- state
		}
	}()

	return parsedCh
}

func read(data []byte, offset int, length int) []byte {
	if offset+length > len(data) {
		return nil
	}
	return data[offset : offset+length]
}

func parseTemperature(data []byte) float64 {
	raw := read(data, 0x2E, 2)
	if raw == nil {
		return 0
	}
	tempRaw := binary.LittleEndian.Uint16(raw)
	return 26 + float64(int16(tempRaw))/127
}

func parseVoltage(data []byte) float64 {
	raw := read(data, 0x2C, 2)
	if raw == nil {
		return 0
	}
	voltageRaw := binary.LittleEndian.Uint16(raw)
	return float64(voltageRaw) * 0.001
}
