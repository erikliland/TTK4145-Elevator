package elev

import (
	. "../channels"
	. "../driver"
	//. "../simulatorCore"
	. "../typedef"
	"log"
	"math"
	"time"
)

const debug = false
const maxSpeed int = 14 //Valid speeds = 0-14
const ElevatorStopDelay = 50 * time.Millisecond

var lampMatrix = [N_FLOORS][3]int{
	{LIGHT_UP1, LIGHT_DOWN1, LIGHT_COMMAND1},
	{LIGHT_UP2, LIGHT_DOWN2, LIGHT_COMMAND2},
	{LIGHT_UP3, LIGHT_DOWN3, LIGHT_COMMAND3},
	{LIGHT_UP4, LIGHT_DOWN4, LIGHT_COMMAND4},
}

var buttonMatrix = [N_FLOORS][3]int{
	{BUTTON_UP1, BUTTON_DOWN1, BUTTON_COMMAND1},
	{BUTTON_UP2, BUTTON_DOWN2, BUTTON_COMMAND2},
	{BUTTON_UP3, BUTTON_DOWN3, BUTTON_COMMAND3},
	{BUTTON_UP4, BUTTON_DOWN4, BUTTON_COMMAND4},
}

type ElevLight struct {
	Type   int
	Floor  int
	Active bool
}

type ElevButton struct {
	Type  int
	Floor int
}

func Init(buttonChannel chan<- ElevButton, lightChannel <-chan ElevLight, motorChannel chan int, floorChannel chan<- int, pollDelay time.Duration) error {
	if err := IOInit(); err != nil {
		log.Println("ELEV:\t IOInit error")
		return err
	}
	resetAllLights()
	go lightController(lightChannel)
	go motorController(motorChannel)
	if getFloorSensor() == -1 {
		motorChannel <- DOWN
		for {
			if getFloorSensor() != -1 {
				motorChannel <- STOP
				break
			} else {
				time.Sleep(pollDelay)
			}
		}
	}
	go readInputs(buttonChannel, pollDelay)
	go readFloorSensor(floorChannel, pollDelay)
	return nil
}

func readInputs(buttonChannel chan<- ElevButton, pollDelay time.Duration) {
	inputMatrix := [N_FLOORS][3]bool{}
	var stopButton bool = false
	for {
		for Type := BUTTON_CALL_UP; Type <= BUTTON_COMMAND; Type++ {
			for Floor := 0; Floor < N_FLOORS; Floor++ {
				tempButton := IO_read_bit(buttonMatrix[Floor][Type])
				if tempButton { //Button pressed
					if !inputMatrix[Floor][Type] { // and first time
						inputMatrix[Floor][Type] = true
						buttonChannel <- ElevButton{Type, Floor}
					}
				} else {
					inputMatrix[Floor][Type] = false
				}
			}
		}
		if IO_read_bit(STOP_BUTTON) {
			if !stopButton {
				stopButton = true
				buttonChannel <- ElevButton{Type: BUTTON_STOP}
			}
		} else {
			stopButton = false
		}
		time.Sleep(pollDelay)
	}
}

func readFloorSensor(floorChannel chan<- int, pollDelay time.Duration) {
	var lastFloor int = -1
	for {
		tempFloor := getFloorSensor()
		if (tempFloor != -1) && (tempFloor != lastFloor) {
			lastFloor = tempFloor
			setFloorIndicator(tempFloor)
			floorChannel <- tempFloor
		}
		time.Sleep(pollDelay)
	}
}

func lightController(lightChannel <-chan ElevLight) {
	var command ElevLight
	for {
		select {
		case command = <-lightChannel:
			switch command.Type {
			case BUTTON_STOP:
				if command.Active {
					IO_set_bit(LIGHT_STOP)
				} else {
					IO_clear_bit(LIGHT_STOP)
				}
			case BUTTON_CALL_UP, BUTTON_CALL_DOWN, BUTTON_COMMAND:
				if command.Active {
					IO_set_bit(lampMatrix[command.Floor][command.Type])
				} else {
					IO_clear_bit(lampMatrix[command.Floor][command.Type])
				}
			case INDICATOR_DOOR:
				if command.Active {
					IO_set_bit(LIGHT_DOOR_OPEN)
				} else {
					IO_clear_bit(LIGHT_DOOR_OPEN)
				}
			default:
				log.Println("ELEV:\t You tried to torch a non-light item")
			}
		}
	}
}

func motorController(motorChannel <-chan int) {
	for {
		select {
		case command := <-motorChannel:
			switch command {
			case STOP:
				time.Sleep(ElevatorStopDelay)
				IO_write_analog(MOTOR, 0)
			case UP:
				IO_clear_bit(MOTORDIR)
				IO_write_analog(MOTOR, 200*int(math.Abs(float64(maxSpeed))))
			case DOWN:
				IO_set_bit(MOTORDIR)
				IO_write_analog(MOTOR, 200*int(math.Abs(float64(maxSpeed))))
			default:
				log.Println("ELEV:\t Invalid motor command: ", command)
			}
		}
	}
}

//---------------SubFunctions-------------------
func setFloorIndicator(floor int) {
	if floor >= N_FLOORS {
		floor = N_FLOORS - 1
		log.Println("ELEV:\t Prøvde å sette etasjelys over", N_FLOORS-1)
	} else if floor < 0 {
		floor = 0
		log.Println("ELEV:\t Prøvde å sette etasjelys under 0")
	}
	if bool((floor & 0x02) != 0) {
		IO_set_bit(LIGHT_FLOOR_IND1)
	} else {
		IO_clear_bit(LIGHT_FLOOR_IND1)
	}
	if bool((floor & 0x01) != 0) {
		IO_set_bit(LIGHT_FLOOR_IND2)
	} else {
		IO_clear_bit(LIGHT_FLOOR_IND2)
	}
}

func getFloorSensor() int {
	if IO_read_bit(SENSOR_FLOOR1) {
		return 0
	} else if IO_read_bit(SENSOR_FLOOR2) {
		return 1
	} else if IO_read_bit(SENSOR_FLOOR3) {
		return 2
	} else if IO_read_bit(SENSOR_FLOOR4) {
		return 3
	} else {
		return -1
	}
}

func resetAllLights() {
	for Type := BUTTON_CALL_UP; Type <= BUTTON_COMMAND; Type++ {
		for Floor := 0; Floor < N_FLOORS; Floor++ {
			IO_clear_bit(lampMatrix[Floor][Type])
		}
	}
	IO_clear_bit(LIGHT_DOOR_OPEN)
	IO_clear_bit(LIGHT_STOP)
}

func printFloorSensors() {
	log.Printf("ELEV:\t FloorSensors: \t0:%v \t1:%v \t2:%v \t3:%v\n",
		IO_read_bit(SENSOR_FLOOR1), IO_read_bit(SENSOR_FLOOR2),
		IO_read_bit(SENSOR_FLOOR3), IO_read_bit(SENSOR_FLOOR4))
}
