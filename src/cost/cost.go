package cost

import (
	. "../typedef"
	"errors"
	"log"
	"sort"
	"strconv"
)

const debug = false
const stopTimeInFloor int = 3
const travelTime int = 2

func AssignNewOrder(knownElevators map[string]*Elevator, activeElevators map[string]bool, externalOrderMatrix [N_FLOORS][2]ElevOrder, Floor, Type int) (string, error) {
	numOfActiveElvators := len(activeElevators)
	printDebug("NumOfActiveElvators" + string(numOfActiveElvators))
	if numOfActiveElvators == 0 {
		return "", errors.New("COST:\t Can not AssignNewOrder with zero active elevators")
	}
	cost := elevCosts{}
	for IP, _ := range activeElevators {
		elevator := ExtendedElevState{knownElevators[IP].State, externalOrderMatrix}
		numOfFloors, numStops := elevator.LengthToOrder(Floor, Type)
		costToOrder := numOfFloors*travelTime + numStops*stopTimeInFloor
		printDebug("Elevator: " + IP + " has cost: " + strconv.Itoa(costToOrder))
		cost = append(cost, elevCost{costToOrder, IP})
	}
	sort.Sort(cost)
	if lowestIP := cost[0].IP; lowestIP != "" {
		log.Println("COST:\t Assigning new order to " + lowestIP)
		return lowestIP, nil
	} else {
		return "", errors.New("COST:\t Something went wrong in AssignNewOrder()")
	}
}

type elevCosts []elevCost

type elevCost struct {
	Cost int
	IP   string
}

func (slice elevCosts) Len() int {
	return len(slice)
}

func (slice elevCosts) Less(i, j int) bool {
	if slice[i].Cost != slice[j].Cost {
		return slice[i].Cost < slice[j].Cost
	}
	return slice[i].IP < slice[j].IP
}

func (slice elevCosts) Swap(i, j int) {
	slice[i], slice[j] = slice[j], slice[i]
}

func (slice elevCosts) Print() {
	for _, e := range slice {
		log.Println("COST:\t IP", e.IP, "has cost ", e.Cost)
	}
}

func printDebug(s string) {
	if debug {
		log.Println("COST:\t", s)
	}
}
