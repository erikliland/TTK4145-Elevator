package simulator

import (
	. "../channels"
	. "../simulatorDef"
	"encoding/json"
	"errors"
	"log"
	"net"
	"strconv"
	"sync"
	"time"
)

const debug = false

type motorCommand struct {
	Speed     int
	Direction int
}

var elevator SimulatorElevator
var elevator_mutex = &sync.Mutex{}
var simulatedMotorChannel = make(chan motorCommand, 3)

//INITIALISATION
func IOInit() error {
	log.Println("SIMULATOR:\t Starting simulator")
	if N_FLOORS != 4 {
		log.Println("SIMULATOR:\t Can´t run the simulator with other than four floors.")
		return errors.New("Could not initialise Simulator with other than 4 floors!")
	}
	elevator_mutex.Lock()
	elevator.LastFloor = 1
	elevator.FloorSensor[elevator.LastFloor] = true
	elevator_mutex.Unlock()
	//Generating localhost adress
	laddr, err := net.ResolveUDPAddr("udp4", "localhost:"+strconv.Itoa(PortFromInterface))
	if err != nil {
		log.Println("SIMULATOR:\t Can not resolve localhost on port: ", PortFromInterface)
		return err
	}

	//Creating local listening connections
	conn, err := net.ListenUDP("udp4", laddr)
	if err != nil {
		log.Println("SIMULATOR:\t Can not create UDP socket on port: ", PortFromInterface)
		return err
	} else {
		log.Println("SIMULATOR:\t Simulator is listening on: ", conn.LocalAddr().String())
	}

	go simulatedMotor()
	go listenForIncommingButtons(conn)
	return nil
}

//MOTOR DYNAMICS
func simulatedMotor() {
	var motorState = S_stoppedAtFloor
	var unfinishedDirection int
	var timeTraveledFromLastFloor time.Duration
	var startedMoving time.Time
	var timer = time.NewTimer(time.Hour)
	timer.Stop()
	for {
		select {
		case command := <-simulatedMotorChannel:
			if debug {
				log.Println("MOTOR:\t Got motor command; speed, dir: ", elevator.MotorSpeed, elevator.Direction)
				log.Println("MOTOR:\t Previus motorstate =", MotorStates[motorState])
				log.Println("MOTOR:\t Waiting on mutex")
			}
			elevator_mutex.Lock()
			if debug {
				log.Println("MOTOR:\t Got mutex")
			}
			switch motorState {
			case S_stoppedBetweenFloors:
				if command.Speed != 0 && command.Direction != 0 {
					if command.Direction == unfinishedDirection {
						startedMoving = time.Now().Add(-timeTraveledFromLastFloor)
						timer.Reset((TravelTimeBetweenFloors_ms - timeTraveledFromLastFloor) * time.Millisecond)
					} else if command.Direction == -unfinishedDirection {
						startedMoving = time.Now().Add(-(TravelTimeBetweenFloors_ms - timeTraveledFromLastFloor))
						timer.Reset(timeTraveledFromLastFloor * time.Millisecond)
					}
					if command.Direction == UP {
						motorState = S_movingUp
					} else {
						motorState = S_movingDown
					}
				} else if debug {
					log.Println("MOTOR:\t Did nothing")
				}
			case S_stoppedAtFloor:
				if command.Speed != 0 && command.Direction != 0 {
					timer.Reset((TravelTimePassingFloor_ms / 2) * time.Millisecond)
					startedMoving = time.Now()
					if elevator.Direction == UP {
						motorState = S_movingUpInsideSensor
						unfinishedDirection = UP
					} else {
						motorState = S_movingDownInsideSensor
						unfinishedDirection = DOWN
					}
				} else if debug {
					log.Println("MOTOR:\t Did nothing")
				}
			case S_movingUp, S_movingDown:
				if command.Speed == 0 {
					timeTraveledFromLastFloor = time.Since(startedMoving)
					motorState = S_stoppedBetweenFloors
				} else if command.Direction == -unfinishedDirection && command.Direction != 0 {
					unfinishedDirection = command.Direction
					timeTraveledFromLastFloor = time.Since(startedMoving)
					timer.Reset(timeTraveledFromLastFloor * time.Millisecond)
					startedMoving = time.Now().Add(timeTraveledFromLastFloor - TravelTimeBetweenFloors_ms)
					if command.Direction == UP {
						motorState = S_movingUp
					} else {
						motorState = S_movingDown
					}
				} else if debug {
					log.Println("MOTOR:\t Did nothing")
				}
			case S_movingUpInsideSensor, S_movingDownInsideSensor:
				if command.Speed == 0 {
					motorState = S_stoppedAtFloor
					timer.Stop()
					unfinishedDirection = 0
					timeTraveledFromLastFloor = 0
				} else if command.Direction == -unfinishedDirection && command.Direction != 0 {
					unfinishedDirection = command.Direction
					timeTraveledFromLastFloor = time.Since(startedMoving)
					timer.Reset(timeTraveledFromLastFloor * time.Millisecond)
					startedMoving = time.Now().Add(timeTraveledFromLastFloor - TravelTimePassingFloor_ms)
					if command.Direction == UP {
						motorState = S_movingUpInsideSensor
					} else {
						motorState = S_movingDownInsideSensor
					}
				} else if debug {
					log.Println("MOTOR:\t Did nothing")
				}
			}
			elevator_mutex.Unlock()
			if debug {
				log.Println("MOTOR:\t Released mutex")
				log.Println("MOTOR:\t New motorstate =", MotorStates[motorState])
				log.Println("MOTOR:\t Done with motor command")
			}
		case <-timer.C:
			if debug {
				log.Println("MOTOR:\t Timer timed out")
				log.Println("MOTOR:\t Previus motorstate =", MotorStates[motorState])
				log.Println("MOTOR:\t Waiting on mutex")
			}
			timer.Stop()
			elevator_mutex.Lock()
			if debug {
				log.Println("MOTOR:\t Got mutex")
			}
			switch motorState {
			case S_movingUp: //Entering sensor from underneath
				motorState = S_movingUpInsideSensor
				startedMoving = time.Now()
				timer.Reset(TravelTimePassingFloor_ms * time.Millisecond)
				elevator.LastFloor++
				elevator.FloorSensor[elevator.LastFloor] = true

			case S_movingDown: //Entering sensor from above
				motorState = S_movingDownInsideSensor
				startedMoving = time.Now()
				timer.Reset(TravelTimePassingFloor_ms * time.Millisecond)
				elevator.LastFloor--
				elevator.FloorSensor[elevator.LastFloor] = true
			case S_movingUpInsideSensor: //Leaving sensor
				if elevator.LastFloor < N_FLOORS-1 {
					motorState = S_movingUp
					startedMoving = time.Now()
					timer.Reset(TravelTimeBetweenFloors_ms * time.Millisecond)
					elevator.FloorSensor[elevator.LastFloor] = false
				} else {
					log.Println("MOTOR:\t Last floor:", elevator.LastFloor)
					log.Fatal("MOTOR:\t You drove the elevator over the top!!!")
				}
			case S_movingDownInsideSensor: //Leaving sensor
				if elevator.LastFloor > 0 {
					motorState = S_movingDown
					startedMoving = time.Now()
					timer.Reset(TravelTimeBetweenFloors_ms * time.Millisecond)
					elevator.FloorSensor[elevator.LastFloor] = false
				} else {
					log.Fatal("MOTOR:\t You drove the elevator under the edge!!!")
				}

			default:
				log.Println("MOTOR:\t Last floor:", elevator.LastFloor)
				log.Println("MOTOR:\t Invalid state at timer timeout!")
			}
			elevator_mutex.Unlock()
			if debug {
				printFloorSensors()
				log.Println("MOTOR:\t Released mutex")
				log.Println("MOTOR:\t Current motorstate =", MotorStates[motorState])
			}
		}
	}
}

func listenForIncommingButtons(conn *net.UDPConn) {
	buf := make([]byte, 1024)
	for {
		n, _, err := conn.ReadFromUDP(buf[:])
		if err != nil {
			log.Println("SIMULATOR:\t Error in UDPConnectionReader")
			log.Fatal(err)
		}
		var command string
		err = json.Unmarshal(buf[:n], &command)
		if err != nil {
			log.Println("SIMULATOR:\t Invalid package from Simulator interface")
			log.Println(err)
		} else {
			if debug {
				log.Println("SIMULATOR:\t Received command: ", command)
			}
			switch command {
			case "q": //UP1
				go simulateButtonPress(&elevator.ButtonMatrix[0][0])
			case "w": //UP2
				go simulateButtonPress(&elevator.ButtonMatrix[1][0])
			case "e": //UP3
				go simulateButtonPress(&elevator.ButtonMatrix[2][0])
			case "s": //DWN2
				go simulateButtonPress(&elevator.ButtonMatrix[1][1])
			case "d": //DWN3
				go simulateButtonPress(&elevator.ButtonMatrix[2][1])
			case "f": //DWN4
				go simulateButtonPress(&elevator.ButtonMatrix[3][1])
			case "z": //OUT1
				go simulateButtonPress(&elevator.ButtonMatrix[0][2])
			case "x": //OUT2
				go simulateButtonPress(&elevator.ButtonMatrix[1][2])
			case "c": //OUT3
				go simulateButtonPress(&elevator.ButtonMatrix[2][2])
			case "v": //OUT4
				go simulateButtonPress(&elevator.ButtonMatrix[3][2])
			case "k": //STOP
				go simulateButtonPress(&elevator.StopButton)
			}
		}
	}
}

//This simulation should be done different to avoid spawning of mulitple threads per button
func simulateButtonPress(button *bool) {
	elevator_mutex.Lock()
	*button = true
	elevator_mutex.Unlock()
	time.Sleep(BtnDepressedTime_ms * time.Millisecond)
	elevator_mutex.Lock()
	*button = false
	elevator_mutex.Unlock()
}

//FUNCTIONS
func IO_set_bit(channel int) {
	elevator_mutex.Lock()
	switch channel {
	case LIGHT_UP1:
		elevator.ButtonLightMatrix[0][0] = true
	case LIGHT_UP2:
		elevator.ButtonLightMatrix[1][0] = true
	case LIGHT_UP3:
		elevator.ButtonLightMatrix[2][0] = true
	case LIGHT_DOWN2:
		elevator.ButtonLightMatrix[1][1] = true
	case LIGHT_DOWN3:
		elevator.ButtonLightMatrix[2][1] = true
	case LIGHT_DOWN4:
		elevator.ButtonLightMatrix[3][1] = true
	case LIGHT_COMMAND1:
		elevator.ButtonLightMatrix[0][2] = true
	case LIGHT_COMMAND2:
		elevator.ButtonLightMatrix[1][2] = true
	case LIGHT_COMMAND3:
		elevator.ButtonLightMatrix[2][2] = true
	case LIGHT_COMMAND4:
		elevator.ButtonLightMatrix[3][2] = true
	case LIGHT_STOP:
		elevator.StopButtonLight = true
	case LIGHT_DOOR_OPEN:
		elevator.DoorOpen = true
	case MOTORDIR:
		elevator.Direction = -1 //Down
		if elevator.MotorSpeed != 0 {
			simulatedMotorChannel <- motorCommand{elevator.MotorSpeed, elevator.Direction}
		}
	case LIGHT_FLOOR_IND1, LIGHT_FLOOR_IND2:

	}
	elevator_mutex.Unlock()
	if debug {
		log.Println("SIMULATOR:\t Setting bit on channel: ", channel)
	}
}

func IO_clear_bit(channel int) {
	elevator_mutex.Lock()
	switch channel {
	case LIGHT_UP1:
		elevator.ButtonLightMatrix[0][0] = false
	case LIGHT_UP2:
		elevator.ButtonLightMatrix[1][0] = false
	case LIGHT_UP3:
		elevator.ButtonLightMatrix[2][0] = false
	case LIGHT_DOWN2:
		elevator.ButtonLightMatrix[1][1] = false
	case LIGHT_DOWN3:
		elevator.ButtonLightMatrix[2][1] = false
	case LIGHT_DOWN4:
		elevator.ButtonLightMatrix[3][1] = false
	case LIGHT_COMMAND1:
		elevator.ButtonLightMatrix[0][2] = false
	case LIGHT_COMMAND2:
		elevator.ButtonLightMatrix[1][2] = false
	case LIGHT_COMMAND3:
		elevator.ButtonLightMatrix[2][2] = false
	case LIGHT_COMMAND4:
		elevator.ButtonLightMatrix[3][2] = false
	case LIGHT_STOP:
		elevator.StopButtonLight = false
	case LIGHT_DOOR_OPEN:
		elevator.DoorOpen = false
	case MOTORDIR:
		elevator.Direction = 1 //UP
		if elevator.MotorSpeed != 0 {
			simulatedMotorChannel <- motorCommand{elevator.MotorSpeed, elevator.Direction}
		}
	case LIGHT_FLOOR_IND1, LIGHT_FLOOR_IND2:
	}
	elevator_mutex.Unlock()
	if debug {
		log.Println("SIMULATOR:\t Clearing bit on channel: ", channel)
	}
}

func IO_write_analog(channel, value int) {
	switch channel {
	case MOTOR:
		elevator_mutex.Lock()
		elevator.MotorSpeed = value
		simulatedMotorChannel <- motorCommand{elevator.MotorSpeed, elevator.Direction}
		elevator_mutex.Unlock()
	}
	if debug {
		log.Printf("Writing %i on channel %i \n", value, channel)
	}
}

func IO_read_bit(channel int) bool {
	switch channel {
	case LIGHT_UP1:
		return elevator.ButtonLightMatrix[0][0]
	case LIGHT_UP2:
		return elevator.ButtonLightMatrix[1][0]
	case LIGHT_UP3:
		return elevator.ButtonLightMatrix[2][0]
	case LIGHT_DOWN2:
		return elevator.ButtonLightMatrix[1][1]
	case LIGHT_DOWN3:
		return elevator.ButtonLightMatrix[2][1]
	case LIGHT_DOWN4:
		return elevator.ButtonLightMatrix[3][1]
	case LIGHT_COMMAND1:
		return elevator.ButtonLightMatrix[0][2]
	case LIGHT_COMMAND2:
		return elevator.ButtonLightMatrix[1][2]
	case LIGHT_COMMAND3:
		return elevator.ButtonLightMatrix[2][2]
	case LIGHT_COMMAND4:
		return elevator.ButtonLightMatrix[3][2]
	case LIGHT_STOP:
		return elevator.StopButtonLight
	case LIGHT_DOOR_OPEN:
		return elevator.DoorOpen
	case BUTTON_UP1:
		return elevator.ButtonMatrix[0][0]
	case BUTTON_UP2:
		return elevator.ButtonMatrix[1][0]
	case BUTTON_UP3:
		return elevator.ButtonMatrix[2][0]
	case BUTTON_DOWN2:
		return elevator.ButtonMatrix[1][1]
	case BUTTON_DOWN3:
		return elevator.ButtonMatrix[2][1]
	case BUTTON_DOWN4:
		return elevator.ButtonMatrix[3][1]
	case BUTTON_COMMAND1:
		return elevator.ButtonMatrix[0][2]
	case BUTTON_COMMAND2:
		return elevator.ButtonMatrix[1][2]
	case BUTTON_COMMAND3:
		return elevator.ButtonMatrix[2][2]
	case BUTTON_COMMAND4:
		return elevator.ButtonMatrix[3][2]
	case STOP_BUTTON:
		return elevator.StopButton
	case OBSTRUCTION:
		return elevator.ObstructionButton
	case SENSOR_FLOOR1:
		return elevator.FloorSensor[0]
	case SENSOR_FLOOR2:
		return elevator.FloorSensor[1]
	case SENSOR_FLOOR3:
		return elevator.FloorSensor[2]
	case SENSOR_FLOOR4:
		return elevator.FloorSensor[3]
	case MOTORDIR:
		if elevator.Direction == 1 {
			return false
		} else {
			return true
		}
	case LIGHT_FLOOR_IND1:
		if (elevator.LastFloor == 1) || (elevator.LastFloor == 3) {
			return true
		} else {
			return false
		}
	case LIGHT_FLOOR_IND2:
		if (elevator.LastFloor == 2) || (elevator.LastFloor == 3) {
			return true
		} else {
			return false
		}
	}
	if debug {
		log.Println("SIMULATOR:\t Reading discrete channel: ", channel)
	}
	return false
}

func IO_read_analog(channel int) int {
	switch channel {
	case MOTOR:
		return elevator.MotorSpeed
	}
	if debug {
		log.Println("SIMULATOR:\t Reading analog channel: ", channel)
	}
	return 0
}

func printFloorSensors() {
	log.Printf("SIMULATOR:\t FloorSensors: \t0:%v \t1:%v \t2:%v \t3:%v\n",
		IO_read_bit(SENSOR_FLOOR1), IO_read_bit(SENSOR_FLOOR2), IO_read_bit(SENSOR_FLOOR3), IO_read_bit(SENSOR_FLOOR4))
}
