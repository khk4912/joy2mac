package joy2mac

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

type MouseOutputConfig struct {
	Sensitivity      float64
	Deadzone         int16
	DistanceDeadzone int16
	JumpThreshold    int16
	Smoothing        float64
	InvertY          bool
}

type MouseMapper struct {
	cfg       MouseOutputConfig
	prevX     int16
	prevY     int16
	hasPrev   bool
	filteredX float64
	filteredY float64
}

var (
	mousePermissionOnce sync.Once
	mousePermissionOK   bool
)

func DefaultMouseOutputConfig() MouseOutputConfig {
	return MouseOutputConfig{
		Sensitivity:      0.2,
		Deadzone:         2,
		DistanceDeadzone: 500,
		JumpThreshold:    1200,
		Smoothing:        0.35,
		InvertY:          false,
	}
}

func NewMouseMapper(cfg MouseOutputConfig) *MouseMapper {
	return &MouseMapper{cfg: cfg}
}

func MouseHandler() {
	if !ensureMousePermission() {
		fmt.Println("macOS permission not granted; mouse output disabled")
		return
	}

	baseCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ctx, cancel := context.WithCancel(baseCtx)
	defer cancel()

	adapterManager := NewAdapterManager(1)
	candidates, err := adapterManager.ScanJoycons()
	if err != nil {
		fmt.Printf("Failed to scan Joy-Con devices: %v\n", err)
		adapterManager.Shutdown()
		return
	}

	if len(candidates) != 1 {
		fmt.Println("Expected 1 Joy-Con device, found", len(candidates))
		fmt.Println("Stopping...")
		adapterManager.Shutdown()
		return
	}

	inputCh := make(chan InputData, 1)
	session := CreateJoyconSession(candidates[0], 1, inputCh)
	if session.Side == UnknownSide {
		session.Side = LeftSide
		fmt.Println("Joy-Con side was unknown; forcing LeftSide for mouse debugging.")
	}
	adapterManager.AddJoyconSession(session)

	err = adapterManager.ConnectSession(session)
	if err != nil {
		fmt.Printf("Failed to connect to Joy-Con at %s: %v\n", candidates[0].AddressString, err)
		adapterManager.Shutdown()
		return
	}

	stateCh := parseInputData(inputCh)

	tuiCh := make(chan JoyconState, 1)
	mouseCh := make(chan JoyconState, 1)

	go fanOutStates(ctx, stateCh, tuiCh, mouseCh)
	go runMouseOutputLoop(ctx, mouseCh)

	runJoyconTUI(ctx, []joyconInputSource{
		{name: "P1", title: "Mouse Mode", stateCh: tuiCh, side: session.Side},
	})
	cancel()

	fmt.Println("\nShutting down...")
	adapterManager.Shutdown()
}

func ensureMousePermission() bool {
	mousePermissionOnce.Do(func() {
		mousePermissionOK = EnsurePostEventPermission(true)
	})

	return mousePermissionOK
}

func runMouseOutputLoop(ctx context.Context, stateCh <-chan JoyconState) {
	if !ensureMousePermission() {
		fmt.Println("macOS permission not granted; mouse output disabled")
		return
	}

	mapper := NewMouseMapper(DefaultMouseOutputConfig())
	var leftDown bool
	var rightDown bool

	for {
		select {
		case <-ctx.Done():
			return
		case state, ok := <-stateCh:
			if !ok {
				return
			}

			nextLeftDown, nextRightDown := mappedMouseButtons(state)
			if nextLeftDown != leftDown {
				MouseLeftButton(nextLeftDown)
				leftDown = nextLeftDown
			}

			if nextRightDown != rightDown {
				MouseRightButton(nextRightDown)
				rightDown = nextRightDown
			}

			dx, dy, ok := mapper.Map(state.Mouse)
			if !ok {
				continue
			}

			switch {
			case leftDown:
				MouseDrag(dx, dy, PointerButtonLeft)
			case rightDown:
				MouseDrag(dx, dy, PointerButtonRight)
			default:
				MouseMove(dx, dy)
			}
		}
	}
}

func fanOutStates(ctx context.Context, in <-chan JoyconState, outs ...chan JoyconState) {
	defer func() {
		for _, out := range outs {
			close(out)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case state, ok := <-in:
			if !ok {
				return
			}

			for _, out := range outs {
				select {
				case <-ctx.Done():
					return
				case out <- state:
				}
			}
		}
	}
}

func (m *MouseMapper) Map(input MouseInput) (dx, dy float64, ok bool) {
	if input.Distance > m.cfg.DistanceDeadzone {
		m.hasPrev = false
		m.filteredX = 0
		m.filteredY = 0
		return 0, 0, false
	}

	if !m.hasPrev {
		m.prevX = input.MouseX
		m.prevY = input.MouseY
		m.hasPrev = true
		return 0, 0, false
	}

	rawDX := input.MouseX - m.prevX
	rawDY := input.MouseY - m.prevY

	m.prevX = input.MouseX
	m.prevY = input.MouseY

	if abs16(rawDX) > m.cfg.JumpThreshold || abs16(rawDY) > m.cfg.JumpThreshold {
		m.filteredX = 0
		m.filteredY = 0
		return 0, 0, false
	}

	if abs16(rawDX) <= m.cfg.Deadzone {
		rawDX = 0
	}
	if abs16(rawDY) <= m.cfg.Deadzone {
		rawDY = 0
	}

	if rawDX == 0 && rawDY == 0 {
		return 0, 0, false
	}

	dx = float64(rawDX) * m.cfg.Sensitivity
	dy = float64(rawDY) * m.cfg.Sensitivity

	m.filteredX = m.cfg.Smoothing*dx + (1-m.cfg.Smoothing)*m.filteredX
	m.filteredY = m.cfg.Smoothing*dy + (1-m.cfg.Smoothing)*m.filteredY

	if m.cfg.InvertY {
		m.filteredY = -m.filteredY
	}

	if m.filteredX == 0 && m.filteredY == 0 {
		return 0, 0, false
	}

	return m.filteredX, m.filteredY, true
}

func abs16(v int16) int16 {
	if v < 0 {
		return -v
	}
	return v
}

func mappedMouseButtons(state JoyconState) (left bool, right bool) {
	switch state.Side {
	case RightSide:
		return state.Buttons[ButtonR], state.Buttons[ButtonZR]
	case LeftSide:
		return state.Buttons[ButtonZL], state.Buttons[ButtonL]
	default:
		return false, false
	}
}
