package joy2mac

import (
	"context"
)

func SingleJoyconHandler(ctx context.Context, inputCh <-chan InputData) {
	stateCh := parseInputData(inputCh)

	runJoyconTUI(ctx, []joyconInputSource{
		{name: "P1", title: "Single Joy-Con", stateCh: stateCh, side: UnknownSide},
	})
}

func DualJoyconHandler(ctx context.Context, leftInputCh <-chan InputData, rightInputCh <-chan InputData) {
	leftStateCh := parseInputData(leftInputCh)
	rightStateCh := parseInputData(rightInputCh)

	runJoyconTUI(ctx, []joyconInputSource{
		{name: "P1", title: "Left Joy-Con 2", stateCh: leftStateCh, side: LeftSide},
		{name: "P2", title: "Right Joy-Con 2", stateCh: rightStateCh, side: RightSide},
	})
}
