(* Loads one pallet on Start: pushes an empty pallet out of the magazine, runs it to the stop,
   sets eight cartons onto it in two layers of four, and runs it to the end of the conveyor.

   Each step sets the outputs it needs, says the step to go to and how long to wait first.
   An emergency stop takes the power from the actuators until it is released and Reset is
   pressed: the chain then starts again from its first step.
   The motors and the go signals of the gantry are off unless a step switches them on; the
   vacuum and the positions the axes are to drive to stay as the steps set them.

   The gantry's positions, by number:
     bridge   (xTarget): 0 pick place, 1 left half of the pallet, 2 right half
     carriage (yTarget): 0 pick place, 1 front row, 2 rear row
     mast     (zTarget): 0 up, 1 pick height, 2 first layer, 3 second layer *)
PROGRAM StepChain
VAR
	xTarget, yTarget, zTarget : INT;   (* where each axis is to go *)
	cartons : INT;                     (* how many cartons are on the pallet *)
	place : INT;                       (* the place of the next carton in its layer, 0 to 3 *)
END_VAR

(* Off unless a step needs them. *)
conveyorRun := FALSE;
stopRelease := FALSE;
pushOut := FALSE;
cartonRun := FALSE;
xGo := FALSE;
yGo := FALSE;
zGo := FALSE;
lStart := FALSE;
lReset := NOT powerOk;
lGreen := currentStep >= 20;
lAmber := magEmpty OR NOT powerOk;

(* Without power, start again from the beginning. *)
IF NOT powerOk THEN
	currentStep := 0;
	nextStep := 0;
END_IF

CASE currentStep OF
	0: (* Init *)
		vacuum := FALSE;
		xTarget := 0;
		yTarget := 0;
		zTarget := 0;
		nextStep := 10;
		stepDelay := T#0ms;

	10: (* Ready: the start lamp asks for Start *)
		lStart := TRUE;
		IF start AND powerOk AND NOT magEmpty AND pusherBack AND NOT palletIn AND NOT palletAtStop AND NOT palletAtEnd THEN
			cartons := 0;
			nextStep := 20;
			stepDelay := T#0ms;
		END_IF

	20: (* Push a pallet out of the magazine *)
		pushOut := TRUE;
		IF pusherOut THEN
			nextStep := 30;
			stepDelay := T#0ms;
		END_IF

	30: (* Pusher back, the pallet on the conveyor *)
		IF pusherBack AND palletIn THEN
			nextStep := 40;
			stepDelay := T#0ms;
		END_IF

	40: (* Run the pallet to the stop, and on until it rests against it *)
		conveyorRun := TRUE;
		IF palletAtStop THEN
			nextStep := 100;
			stepDelay := T#3s;
		END_IF

	100: (* Bring a carton to the pick place *)
		cartonRun := TRUE;
		IF cartonAtPick THEN
			nextStep := 110;
			stepDelay := T#1s;
		END_IF

	110: (* Bridge and carriage over the carton: the positions first, the start a moment
	        later, as a drive takes the position number when its go signal comes on *)
		xTarget := 0;
		yTarget := 0;
		nextStep := 115;
		stepDelay := T#100ms;

	115:
		xGo := TRUE;
		yGo := TRUE;
		nextStep := 120;
		stepDelay := T#200ms;

	120:
		xGo := TRUE;
		yGo := TRUE;
		IF xDone AND yDone THEN
			nextStep := 130;
			stepDelay := T#0ms;
		END_IF

	130: (* Mast down onto the carton *)
		zTarget := 1;
		nextStep := 135;
		stepDelay := T#100ms;

	135:
		zGo := TRUE;
		nextStep := 140;
		stepDelay := T#200ms;

	140:
		zGo := TRUE;
		IF zDone THEN
			vacuum := TRUE;
			nextStep := 150;
			stepDelay := T#0ms;
		END_IF

	150: (* Holding it: mast up *)
		IF vacuumOk THEN
			zTarget := 0;
			nextStep := 160;
			stepDelay := T#100ms;
		END_IF

	160:
		zGo := TRUE;
		nextStep := 170;
		stepDelay := T#200ms;

	170:
		zGo := TRUE;
		IF zDone THEN
			nextStep := 180;
			stepDelay := T#0ms;
		END_IF

	180: (* Over its place on the pallet *)
		place := cartons MOD 4;
		xTarget := 1 + place MOD 2;
		yTarget := 1 + place / 2;
		nextStep := 185;
		stepDelay := T#100ms;

	185:
		xGo := TRUE;
		yGo := TRUE;
		nextStep := 190;
		stepDelay := T#200ms;

	190:
		xGo := TRUE;
		yGo := TRUE;
		IF xDone AND yDone THEN
			nextStep := 200;
			stepDelay := T#0ms;
		END_IF

	200: (* Down to its layer, and let go *)
		zTarget := 2 + cartons / 4;
		nextStep := 205;
		stepDelay := T#100ms;

	205:
		zGo := TRUE;
		nextStep := 210;
		stepDelay := T#200ms;

	210:
		zGo := TRUE;
		IF zDone THEN
			vacuum := FALSE;
			nextStep := 220;
			stepDelay := T#300ms;
		END_IF

	220: (* Mast up again *)
		zTarget := 0;
		nextStep := 225;
		stepDelay := T#100ms;

	225:
		zGo := TRUE;
		nextStep := 230;
		stepDelay := T#200ms;

	230:
		zGo := TRUE;
		IF zDone THEN
			cartons := cartons + 1;
			IF cartons < 8 THEN
				nextStep := 100;
			ELSE
				nextStep := 300;
			END_IF
			stepDelay := T#0ms;
		END_IF

	300: (* The pallet is full: lower the stop and run it to the end, until it rests against
	        the bumper *)
		stopRelease := TRUE;
		conveyorRun := TRUE;
		IF palletAtEnd THEN
			nextStep := 10;
			stepDelay := T#3s;
		END_IF

END_CASE

(* The positions the axes are to go to, as numbers in binary. *)
xPos0 := xTarget MOD 2 = 1;
xPos1 := xTarget >= 2;
yPos0 := yTarget MOD 2 = 1;
yPos1 := yTarget >= 2;
zPos0 := zTarget MOD 2 = 1;
zPos1 := zTarget >= 2;
END_PROGRAM
