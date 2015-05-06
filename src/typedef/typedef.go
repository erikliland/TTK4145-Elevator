package typedef

import (
	"fmt"
	"log"
	"reflect"
	"time"
)

const N_FLOORS int = 4

//Motor commands
const UP = 1
const STOP = 0
const DOWN = -1

var MotorCommands = [3]string{
	"DOWN",
	"STOP",
	"UP",
}

//Enumerator
const (
	BUTTON_CALL_UP = iota
	BUTTON_CALL_DOWN
	BUTTON_COMMAND
	SENSOR_FLOOR
	INDICATOR_FLOOR
	BUTTON_STOP
	SENSOR_OBST
	INDICATOR_DOOR
)

const (
	EvIAmAlive = iota
	EvBackupState
	EvRequestingState
	EvRestoredStateReturned
	EvNewOrder
	EvAckNewOrder
	EvOrderConfirmed
	EvAckOrderConfirmed
	EvOrderDone
	EvAckOrderDone
	EvReassignOrder
)

const ( //ElevOrder status
	NotActive = iota
	Awaiting
	UnderExecution
)

var ElevOrderStatus = []string{
	"NotActive",
	"Awaiting",
	"UnderExecution",
}

var ButtonType = []string{
	"BUTTON_CALL_UP",
	"BUTTON_CALL_DOWN",
	"BUTTON_COMMAND",
	"SENSOR_FLOOR",
	"INDICATOR_FLOOR",
	"BUTTON_STOP",
	"SENSOR_OBST",
	"INDICATOR_DOOR",
}

var EventType = []string{
	"EvIAmAlive",
	"EvBackupState",
	"EvRequestingState",
	"EvRestoredStateReturned",
	"EvNewOrder",
	"EvAckNewOrder",
	"EvOrderConfirmed",
	"EvAckOrderConfirmed",
	"EvOrderDone",
	"EvAckOrderDone",
	"EvReassignOrder",
}

//------------DATA TYPES-------
type ElevState struct {
	LocalIP        string
	LastFloor      int
	Direction      int
	IsMoving       bool
	DoorIsOpen bool
	InternalOrders [N_FLOORS]bool
}

type ExtendedElevState struct {
	LocalState     ElevState
	ExternalOrders [N_FLOORS][2]ElevOrder
}

type ElevOrder struct {
	Status      int
	AssignedTo  string
	ConfirmedBy map[string]bool
	Timer       *time.Timer `json:"-"`
}

type ExtendedElevOrder struct {
	Floor, Type int
	Order       ElevOrder
	OriginIP    string
}

type ElevOrderMessage struct {
	Floor      int
	ButtonType int
	AssignedTo string
	OriginIP   string
	SenderIP   string
	Event      int
}

type ElevRestoreMessage struct {
	AskerIP             string
	ResponderIP         string
	Event               int
	State               ElevState
	ExternalOrderMatrix [N_FLOORS][2]ElevOrder
}

type Elevator struct {
	State ElevState
	Time  time.Time
}

//-------------HELP FUNCTIONS --------------------

//Resolve
func ResolveIAmAliveMessage(elev *Elevator) ElevRestoreMessage {
	return ElevRestoreMessage{ResponderIP: elev.State.LocalIP, Event: EvIAmAlive, State: elev.State}
}

func ResolveBackupState(elev *Elevator, externalOrderMatrix [N_FLOORS][2]ElevOrder) ElevRestoreMessage {
	return ElevRestoreMessage{ResponderIP: elev.State.LocalIP, State: elev.State, Event: EvBackupState, ExternalOrderMatrix: externalOrderMatrix}
}

func ResolveElevator(state ElevState) *Elevator {
	return &Elevator{state, time.Now()}
}

//TYPE *Elevator
func (elev *Elevator) Print() {
	elev.State.Print()
}

func (elev *Elevator) ResolveExtendedElevState(externalOrderMatrix [N_FLOORS][2]ElevOrder) ExtendedElevState {
	return ExtendedElevState{elev.State, externalOrderMatrix}
}

func (elev *Elevator) MergeStates(restoredState ElevState) bool {
	if len(restoredState.InternalOrders) == len(elev.State.InternalOrders) {
		for i, order := range restoredState.InternalOrders {
			elev.State.InternalOrders[i] = elev.State.InternalOrders[i] || order
		}
		return true
	} else {
		return false
	}
}

func (elev *Elevator) SetInternalOrder(floor int) {
	elev.State.InternalOrders[floor] = true
}

func (elev *Elevator) ClearInternalOrder(floor int) {
	elev.State.InternalOrders[floor] = false
}

func (elev *Elevator) ClearInternalOrderAtCurrentFloor() {
	elev.State.InternalOrders[elev.State.LastFloor] = false
}

func (elev *Elevator) SetLastFloor(floor int) {
	elev.State.LastFloor = floor
}

func (elev *Elevator) SetDirection(direction int) {
	elev.State.Direction = direction
}

func (elev *Elevator) SetMoving(moving bool) {
	elev.State.IsMoving = moving
}

func (elev *Elevator) IsIdle() bool {
	return !elev.State.IsMoving && elev.State.Direction == STOP
}

//TYPE ElevOrder
func (o *ElevOrder) StopTimer() bool {
	if o.Timer != nil {
		return o.Timer.Stop()
	}
	return false
}

func (order *ElevOrder) DeleteConfirmedBy() {
	for key := range order.ConfirmedBy {
		delete(order.ConfirmedBy, key)
	}
	order.ConfirmedBy = make(map[string]bool)
}

func (o ElevOrder) Print() {
	fmt.Println("Status:\t", o.Status)
	fmt.Println("AssignedTo:\t", o.AssignedTo)
	fmt.Println("ConfirmedBy:\t", o.ConfirmedBy)
}

//TYPE ExtendedElevOrder
func (o ExtendedElevOrder) Print() {
	fmt.Println("ExtendedElevOrder")
	fmt.Println("Floor:\t\t", o.Floor)
	fmt.Println("Type:\t\t", ButtonType[o.Type])
	fmt.Println("AssignedTo:\t", o.Order.AssignedTo)
}

//TYPE ElevState
func (s ElevState) Print() {
	fmt.Println("ElevState to:\t ", s.LocalIP)
	fmt.Println("LastFloor:\t ", s.LastFloor)
	fmt.Println("Direction:\t ", s.Direction)
	fmt.Printf("Internal orders: %v\n", s.InternalOrders)
}

//TYPE ElevOrderMessage
func (m ElevOrderMessage) Print() {
	fmt.Println("ElevOrderMessage")
	fmt.Println("SenderIP:\t", m.SenderIP)
	fmt.Println("OriginIP:\t", m.OriginIP)
	fmt.Println("AssignedTo:\t", m.AssignedTo)
	fmt.Println("Event:\t\t", EventType[m.Event])
	fmt.Println("ButtonType:\t", ButtonType[m.ButtonType])
	fmt.Println("Floor:\t\t", m.Floor)
}

func (m ElevOrderMessage) IsValid() bool {
	if m.Floor > N_FLOORS || m.Floor < -1 {
		return false
	}
	if m.ButtonType > 2 || m.ButtonType < 0 {
		return false
	}
	if m.Event > 10 || m.Event < 4 {
		return false
	}
	return true
}

//TYPE ElevRestoreMessage
func (m ElevRestoreMessage) Print() {
	fmt.Println("Event:\t\t", EventType[m.Event])
	fmt.Println("AskerIP:\t", m.AskerIP)
	fmt.Println("ResponderIP:\t", m.ResponderIP)
	if !reflect.DeepEqual(m.State, ExtendedElevState{}) {
		m.State.Print()
	} else {
		fmt.Println("State:\t N/A")
	}
}

func (m ElevRestoreMessage) IsValid() bool {
	if m.AskerIP == m.ResponderIP {
		return false
	}
	if m.Event > 3 || m.Event < 0 {
		return false
	}
	return true
}

//TYPE ExtendedElevState
func (s ExtendedElevState) Print() {
	s.LocalState.Print()
	//s.ExternalOrders.Print()
}

func (e ExtendedElevState) IsIdle() bool {
	return !e.LocalState.IsMoving &&
		e.LocalState.Direction == STOP &&
		!e.HaveOrders()
}

func (s ExtendedElevState) LengthToOrder(orderFloor, orderType int) (int, int) {
	localIP := s.LocalState.LocalIP
	dir := s.LocalState.Direction
	lastFloor := s.LocalState.LastFloor
	numbersOfFloors := 0
	numbersOfStops := 0

	if dir == STOP && !s.LocalState.IsMoving && lastFloor == orderFloor { //Is idle
		return 0, 0
	}

	if orderFloor > lastFloor {
		if !(dir == DOWN && s.HaveOrdersBelow()) {
			dir = UP
		}
	} else if orderFloor < lastFloor {
		if !(dir == UP && s.HaveOrdersAbove()) {
			dir = DOWN
		}
	}

	for floor := lastFloor + dir; floor < N_FLOORS && floor >= 0; floor += dir {
		numbersOfFloors++
		if floor == orderFloor {
			if floor == 0 || floor == N_FLOORS-1 {
				return numbersOfFloors, numbersOfStops
			} else if (dir == DOWN && orderType == BUTTON_CALL_DOWN) ||
				(dir == UP && orderType == BUTTON_CALL_UP) ||
				(orderType == BUTTON_COMMAND) {
				return numbersOfFloors, numbersOfStops
			} else {
				fakeElev := s.createFakeElev(orderFloor)
				if dir == UP && !fakeElev.HaveOrdersAbove() {
					return numbersOfFloors, numbersOfStops
				} else if dir == DOWN && !fakeElev.HaveOrdersBelow() {
					return numbersOfFloors, numbersOfStops
				}
			}
		}
		for button := BUTTON_CALL_UP; button == BUTTON_CALL_DOWN || button == BUTTON_CALL_UP; button++ {
			if s.ExternalOrders[floor][button].Status == UnderExecution &&
				localIP == s.ExternalOrders[floor][button].AssignedTo {
				numbersOfStops++
				break
			} else if s.LocalState.InternalOrders[floor] {
				numbersOfStops++
				break
			}
		}

		if floor == N_FLOORS-1 {
			dir = DOWN
		} else if floor == 0 {
			dir = UP
		}
	}
	return numbersOfFloors, numbersOfStops
}

func (s ExtendedElevState) ShouldStop() bool {
	localIP := s.LocalState.LocalIP
	floor := s.LocalState.LastFloor
	switch s.LocalState.Direction {
	case STOP:
		return true
	case UP:
		return !s.HaveOrdersAbove() ||
			s.LocalState.InternalOrders[floor] ||
			(s.ExternalOrders[floor][BUTTON_CALL_UP].Status == UnderExecution && s.ExternalOrders[floor][BUTTON_CALL_UP].AssignedTo == localIP) ||
			s.LocalState.InternalOrders[floor] ||
			floor == N_FLOORS-1
	case DOWN:
		return !s.HaveOrdersBelow() ||
			s.LocalState.InternalOrders[floor] ||
			(s.ExternalOrders[floor][BUTTON_CALL_DOWN].Status == UnderExecution && s.ExternalOrders[floor][BUTTON_CALL_DOWN].AssignedTo == localIP) ||
			floor == 0
	}
	log.Fatal("MAIN:\t iShouldStop was run with an invalid elev.State.Direction")
	return true
}

func (s ExtendedElevState) HaveOrdersAbove() bool {
	localIP := s.LocalState.LocalIP
	for floor := N_FLOORS - 1; floor > s.LocalState.LastFloor; floor-- {
		if s.LocalState.InternalOrders[floor] {
			return true
		}
		for _, order := range s.ExternalOrders[floor] {
			if order.Status == UnderExecution && order.AssignedTo == localIP {
				return true
			}
		}
	}
	return false
}

func (s ExtendedElevState) HaveOrdersBelow() bool {
	localIP := s.LocalState.LocalIP
	for floor := 0; floor < s.LocalState.LastFloor; floor++ {
		if s.LocalState.InternalOrders[floor] {
			return true
		}
		for _, order := range s.ExternalOrders[floor] {
			if order.Status == UnderExecution && order.AssignedTo == localIP {
				return true
			}
		}
	}
	return false
}

func (s ExtendedElevState) HaveOrdersAtCurrentFloor() bool {
	floor := s.LocalState.LastFloor
	localIP := s.LocalState.LocalIP
	if s.LocalState.InternalOrders[floor] {
		return true
	}
	for _, order := range s.ExternalOrders[floor] {
		if order.Status == UnderExecution && order.AssignedTo == localIP {
			return true
		}
	}
	return false
}

func (s ExtendedElevState) HaveOrders() bool {

	return s.HaveOrdersAbove() || s.HaveOrdersBelow() || s.HaveOrdersAtCurrentFloor()
}

func (s ExtendedElevState) createFakeElev(floor int) ExtendedElevState {
	temp := s
	temp.LocalState.LastFloor = floor
	return temp
}

func (s ExtendedElevState) GetNextDirection() int {
	if !s.HaveOrders() {
		return STOP
	}
	switch s.LocalState.Direction {
	case UP:
		if s.HaveOrdersAbove() && s.LocalState.LastFloor != N_FLOORS-1 {
			return UP
		}
		fallthrough
	case DOWN:
		if s.HaveOrdersBelow() && s.LocalState.LastFloor != 0 {
			return DOWN
		}
		fallthrough
	case STOP:
		if s.HaveOrdersAbove() {
			return UP
		} else if s.HaveOrdersBelow() {
			return DOWN
		}
	}
	return STOP
}

func (e ExtendedElevState) GetNextButtonDirection() int {
	dir := e.GetNextDirection()
	if dir == UP {
		return BUTTON_CALL_UP
	}
	if dir == DOWN {
		return BUTTON_CALL_DOWN
	}
	return -1
}

func (s ExtendedElevState) FindExternalOrdersAtCurrentFloor() []ExtendedElevOrder {
	localIP := s.LocalState.LocalIP
	list := []ExtendedElevOrder{}
	floor := s.LocalState.LastFloor
	if o := s.ExternalOrders[floor][BUTTON_CALL_UP]; o.Status == UnderExecution && o.AssignedTo == localIP {
		list = append(list, ExtendedElevOrder{
			Floor: floor,
			Type:  BUTTON_CALL_UP,
			Order: s.ExternalOrders[floor][BUTTON_CALL_UP],
		})
	}
	if o := s.ExternalOrders[floor][BUTTON_CALL_DOWN]; o.Status == UnderExecution && o.AssignedTo == localIP {
		list = append(list, ExtendedElevOrder{
			Floor: floor,
			Type:  BUTTON_CALL_DOWN,
			Order: s.ExternalOrders[floor][BUTTON_CALL_DOWN],
		})
	}
	return list
}
