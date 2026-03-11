package joy2mac

type JoyconSide int

const (
	UnknownSide JoyconSide = iota
	LeftSide
	RightSide
)

type Button int

const (
	ButtonUnknown Button = iota
	ButtonUp
	ButtonDown
	ButtonLeft
	ButtonRight
	ButtonA
	ButtonB
	ButtonX
	ButtonY
	ButtonL
	ButtonR
	ButtonZL
	ButtonZR
	ButtonSL
	ButtonSR
	ButtonPlus
	ButtonMinus
	ButtonHome
	ButtonCapture
	ButtonStick
	ButtonGameChat
)

type StickInput struct {
	X int16
	Y int16
}

type InputData struct {
	PlayerNo int
	Side     JoyconSide
	Data     []byte
}

type ButtonState map[Button]bool

type JoyconState struct {
	PlayerNo    int
	Side        JoyconSide
	Raw         []byte
	Stick       StickInput
	Buttons     ButtonState
	Temperature float64
	Accel       [3]float64 // X Y Z
	Gyro        [3]float64
	Voltage     float64
	Ampere      float64
	Mouse       MouseInput
}

type MouseInput struct {
	MouseX   int16
	MouseY   int16
	Distance int16
}

var State JoyconState
