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
				Accel:       parseAccel(input.Data),
				Gyro:        parseGyro(input.Data),
				Voltage:     parseVoltage(input.Data),
			}

			switch state.Side {
			case LeftSide:
				// state.Stick = parseLeftStick(input.Data)
				state.Buttons = parseLeftButtons(input.Data)
			case RightSide:
				// state.Buttons = parseRightButtons(input.Data)
				// state.Stick = parseRightStick(input.Data)
			}

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

	// 25°C + raw / 127
	return 26 + float64(int16(tempRaw))/127
}

func parseVoltage(data []byte) float64 {
	raw := read(data, 0x1C, 2)
	if raw == nil {
		return 0
	}
	voltageRaw := binary.LittleEndian.Uint16(raw)

	return float64(voltageRaw) * 0.001
}

func parseAccel(data []byte) [3]float64 {
	raw := read(data, 0x30, 6)

	Xraw := binary.LittleEndian.Uint16(raw[0:2])
	Yraw := binary.LittleEndian.Uint16(raw[2:4])
	Zraw := binary.LittleEndian.Uint16(raw[4:6])

	// 4096 equals 1G, returns acceleration in G
	return [3]float64{
		float64(int16(Xraw)) / 4096,
		float64(int16(Yraw)) / 4096,
		float64(int16(Zraw)) / 4096,
	}
}

func parseGyro(data []byte) [3]float64 {
	// 48000 = 360°/s, returns angular velocity in °/s

	raw := read(data, 0x36, 6)

	Xraw := binary.LittleEndian.Uint16(raw[0:2])
	Yraw := binary.LittleEndian.Uint16(raw[2:4])
	Zraw := binary.LittleEndian.Uint16(raw[4:6])

	return [3]float64{
		float64(int16(Xraw)) / 48000 * 360,
		float64(int16(Yraw)) / 48000 * 360,
		float64(int16(Zraw)) / 48000 * 360,
	}
}

func parseLeftButtons(data []byte) ButtonState {
	rawButtons := read(data, 3, 4)
	parsedNum := binary.LittleEndian.Uint32(rawButtons)

	return ButtonState{
		ButtonDown:  parsedNum&1 == 1,
		ButtonUp:    parsedNum&2 == 2,
		ButtonRight: parsedNum&4 == 4,
		ButtonLeft:  parsedNum&8 == 8,
		ButtonSR:    parsedNum&0x10 == 0x10,
		ButtonSL:    parsedNum&0x20 == 0x20,
		ButtonL:     parsedNum&0x40 == 0x40,
		ButtonZL:    parsedNum&0x80 == 0x80,
		ButtonMinus: parsedNum&0x100 == 0x100,
		ButtonStick: parsedNum&0x800 == 0x800,
	}
}
