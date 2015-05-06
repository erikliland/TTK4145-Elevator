package main

import (
	"./src/cost"
	"./src/elev"
	"./src/network"
	. "./src/typedef"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"time"
)

const debug = false

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	const connectionAttempsLimit = 10
	const iAmAliveTickTime = 100 * time.Millisecond
	const iAmAliveLimit = 3*iAmAliveTickTime + 10*time.Millisecond
	const ackTimeout = 500 * time.Millisecond
	const doorWaitTime = 3000 * time.Millisecond
	const elevatorPollDelay = 50 * time.Millisecond
	var orderTimeout = 5*time.Second + time.Duration(r.Intn(2000))*time.Millisecond
	var localIP string
	var externalOrderMatrix [N_FLOORS][2]ElevOrder
	var knownElevators = make(map[string]*Elevator) //key = IPadr
	var activeElevators = make(map[string]bool)     //key = IPadr

	//-----Initialise hardware------
	log.Println("MAIN:\t Starting main")
	buttonChannel := make(chan elev.ElevButton, 10)
	lightChannel := make(chan elev.ElevLight)
	motorChannel := make(chan int)
	floorChannel := make(chan int)
	if err := elev.Init(buttonChannel, lightChannel, motorChannel, floorChannel, elevatorPollDelay); err != nil {
		log.Println("MAIN:\t Hardware init failed!")
		log.Fatal(err)
	} else {
		printDebug("Hardware init successful!")
	}

	//-----Initialise monkey handling------
	killChan := make(chan os.Signal)
	signal.Notify(killChan, os.Interrupt)
	go func() {
		<-killChan
		motorChannel <- STOP
		fmt.Println("\n---------------------         SOMEBODY KILLED THIS ELEVATOR!         ---------------------")
		time.Sleep(100 * time.Millisecond)
		os.Exit(1)
	}()

	//-----Initialise network------
	receiveOrderChannel := make(chan ElevOrderMessage, 5)
	sendOrderChannel := make(chan ElevOrderMessage)
	receiveRestoreChannel := make(chan ElevRestoreMessage, 5)
	sendRestoreChannel := make(chan ElevRestoreMessage)
	localIP, err := initNetwork(connectionAttempsLimit, receiveOrderChannel, sendOrderChannel, receiveRestoreChannel, sendRestoreChannel)
	if err != nil {
		log.Println("MAIN:\t Network init failed")
		log.Fatal(err)
	} else if debug {
		log.Println("MAIN:\t Network init successful")
	}

	//-----Initialise state------
	log.Println("MAIN:\t Sending out a request after my previus state")
	sendRestoreChannel <- ElevRestoreMessage{
		AskerIP: localIP,
		State:   ElevState{},
		Event:   EvRequestingState,
	}
	knownElevators[localIP] = ResolveElevator(ElevState{LocalIP: localIP, LastFloor: <-floorChannel})
	updateActiveElevators(knownElevators, activeElevators, localIP, iAmAliveLimit)
	log.Println("MAIN:\t State init finished. Starting from floor:", knownElevators[localIP].State.LastFloor)

	//-----Initialise timer------
	checkAliveTick := time.NewTicker(iAmAliveLimit)
	defer checkAliveTick.Stop()
	iAmAliveTick := time.NewTicker(iAmAliveTickTime)
	defer iAmAliveTick.Stop()
	doorTimer := time.NewTimer(time.Second)
	doorTimer.Stop()
	defer doorTimer.Stop()
	timeoutChannel := make(chan ExtendedElevOrder)
	log.Println("MAIN:\t Ticker and timer init successful")

	//------Run------------
	log.Println("MAIN:\t Starting event loop")
	fmt.Println("----------------------------------------------------------------------------------------------------------")
	for {
		select {
		//------------------------------------NETWORK------------------------------------------------
		//------STATE RESTORE AND BACKUP-----------
		case msg := <-receiveRestoreChannel:
			switch msg.Event {
			case EvIAmAlive:
				if _, ok := knownElevators[msg.ResponderIP]; ok {
					knownElevators[msg.ResponderIP].Time = time.Now()
				} else {
					printDebug("Recived EvIAmAlive from a new elevator with IP " + msg.ResponderIP)
					knownElevators[msg.ResponderIP] = ResolveElevator(msg.State)
				}
				updateActiveElevators(knownElevators, activeElevators, localIP, iAmAliveLimit)

			case EvBackupState:
				for floor := 0; floor < N_FLOORS; floor++ {
					for button := 0; button < 2; button++ {
						if externalOrderMatrix[floor][button].Status == UnderExecution && externalOrderMatrix[floor][button].AssignedTo == msg.ResponderIP {
							if externalOrderMatrix[floor][button].AssignedTo == localIP {
								printDebug("Refreshing order execution timer on my order " + ButtonType[button] + " on floor " + strconv.Itoa(floor))
								externalOrderMatrix[floor][button].Timer.Reset(orderTimeout)
							} else {
								printDebug("Refreshing order execution timer on order " + ButtonType[button] + " on floor " + strconv.Itoa(floor))
								externalOrderMatrix[floor][button].Timer.Reset(2 * orderTimeout)
							}

						}
					}
				}
				if msg.ResponderIP != localIP {
					if msg.ResponderIP == msg.State.LocalIP {
						if _, ok := knownElevators[msg.ResponderIP]; ok {
							knownElevators[msg.ResponderIP].State = msg.State
						} else {
							printDebug("Recived EvBackupState from an unknown elevator with IP " + msg.ResponderIP)
							knownElevators[msg.ResponderIP] = ResolveElevator(msg.State)
						}
						knownElevators[msg.ResponderIP].Time = time.Now()
						updateActiveElevators(knownElevators, activeElevators, localIP, iAmAliveLimit)
					} else {
						printDebug("Recived EvBackupState with an inconsisten IP. Rejecting...")
					}
				}

			case EvRequestingState:
				if msg.AskerIP != localIP {
					log.Println("MAIN:\t Received an ElevRestoreMessage from from:", msg.AskerIP)
					if _, ok := knownElevators[msg.AskerIP]; ok {
						log.Println("MAIN:\t I have a stored state for this elevator. Returning the stored state.....")
						sendRestoreChannel <- ElevRestoreMessage{
							Event:               EvRestoredStateReturned,
							AskerIP:             msg.AskerIP,
							ResponderIP:         localIP,
							State:               knownElevators[msg.AskerIP].State,
							ExternalOrderMatrix: externalOrderMatrix,
						}
					} else {
						log.Println("MAIN:\t I do not have a stored state for this elevator.")
					}
				}
			case EvRestoredStateReturned:
				if msg.AskerIP == localIP {
					log.Println("MAIN:\t This ElevRestoreMessage is for me!")
					log.Println(msg.ExternalOrderMatrix)
					for floor, ordersAtFloor := range msg.ExternalOrderMatrix {
						for buttonType, order := range ordersAtFloor {
							if order.Status == UnderExecution && order.AssignedTo != localIP && externalOrderMatrix[floor][buttonType].Status == NotActive {
								printDebug("Adding external order " + ButtonType[buttonType] + " on floor " + strconv.Itoa(floor))
								lightChannel <- elev.ElevLight{Type: buttonType, Floor: floor, Active: true}
								externalOrderMatrix[floor][buttonType].Status = UnderExecution
								externalOrderMatrix[floor][buttonType].DeleteConfirmedBy()
								externalOrderMatrix[floor][buttonType].AssignedTo = order.AssignedTo
								externalOrderMatrix[floor][buttonType].Timer = time.AfterFunc(2*orderTimeout, func() {
									log.Println("TIMEOUT:\t An order under execution timed out.")
									timeoutChannel <- ExtendedElevOrder{
										Floor: floor,
										Type:  buttonType,
										Order: externalOrderMatrix[floor][buttonType],
									}
								})
							}
						}
					}
					if changes := knownElevators[localIP].MergeStates(msg.State); changes {
						for floor, status := range knownElevators[localIP].State.InternalOrders {
							lightChannel <- elev.ElevLight{Floor: floor, Type: BUTTON_COMMAND, Active: status}
						}
						if knownElevators[localIP].IsIdle() && !knownElevators[localIP].State.DoorIsOpen {
							doorTimer.Reset(0 * time.Millisecond)
						}
					}
				} else {
					printDebug("This ElevRestoreMessage is NOT for me!")
				}
			default:
				printDebug("Recived an invalid ElevRestoreMessage from " + msg.ResponderIP)
			}

		//----------ORDERS------------
		case msg := <-receiveOrderChannel:
			printDebug("Received an " + EventType[msg.Event] + " from " + msg.SenderIP + " with OriginIP " + msg.OriginIP)
			switch msg.Event {
			case EvNewOrder:
				switch externalOrderMatrix[msg.Floor][msg.ButtonType].Status {
				case NotActive:
					printDebug("Order " + ButtonType[msg.ButtonType] + " on floor " + strconv.Itoa(msg.Floor) + " assignedTo " + msg.AssignedTo)
					printDebug("The order has status NotActive. Setting it to Awaiting.")
					externalOrderMatrix[msg.Floor][msg.ButtonType].Status = Awaiting
					externalOrderMatrix[msg.Floor][msg.ButtonType].AssignedTo = msg.AssignedTo
					externalOrderMatrix[msg.Floor][msg.ButtonType].DeleteConfirmedBy()
					if msg.OriginIP == localIP {
						printDebug("Starting timeoutTimer [EvAckNewOrder] on order" + ButtonType[msg.ButtonType] + " on floor " + strconv.Itoa(msg.Floor))
						externalOrderMatrix[msg.Floor][msg.ButtonType].Timer = time.AfterFunc(ackTimeout, func() {
							log.Println("TIMEOUT:\t A newOrder was not ack´d by all activeElevators.")
							timeoutChannel <- ExtendedElevOrder{
								Floor: msg.Floor,
								Type:  msg.ButtonType,
								Order: ElevOrder{
									Status:     NotActive,
									AssignedTo: msg.AssignedTo,
									Timer:      externalOrderMatrix[msg.Floor][msg.ButtonType].Timer,
								},
							}
						})
					}
					sendOrderChannel <- ElevOrderMessage{
						Floor:      msg.Floor,
						ButtonType: msg.ButtonType,
						AssignedTo: msg.AssignedTo,
						OriginIP:   msg.OriginIP,
						SenderIP:   localIP,
						Event:      EvAckNewOrder,
					}
				case Awaiting:
					printDebug("Received an EvNewOrder which is already Awaiting")

				case UnderExecution:
					printDebug("Received an EvNewOrder which is already UnderExecution.")
				}

			case EvAckNewOrder:
				if msg.OriginIP == localIP {
					switch externalOrderMatrix[msg.Floor][msg.ButtonType].Status {
					case Awaiting:
						externalOrderMatrix[msg.Floor][msg.ButtonType].ConfirmedBy[msg.SenderIP] = true
						if allActiveElevatorsHaveAcked(externalOrderMatrix, activeElevators, msg) {
							printDebug("Recived AckNewOrder from all active elevators. Sending orderConfirmed")
							printDebug("Stoping timeoutTimer [EvAckNewOrder] on order " + ButtonType[msg.ButtonType] + " on floor " + strconv.Itoa(msg.Floor))
							externalOrderMatrix[msg.Floor][msg.ButtonType].StopTimer()
							externalOrderMatrix[msg.Floor][msg.ButtonType].DeleteConfirmedBy()
							printDebug("Starting timeoutTimer [EvAckOrderConfirmed] on order" + ButtonType[msg.ButtonType] + "on floor" + strconv.Itoa(msg.Floor))
							externalOrderMatrix[msg.Floor][msg.ButtonType].Timer = time.AfterFunc(ackTimeout, func() {
								log.Println("TIMEOUT:\t An orderConfirmed was not ack´d by all activeElevators.")
								timeoutChannel <- ExtendedElevOrder{
									Floor:    msg.Floor,
									Type:     msg.ButtonType,
									OriginIP: msg.OriginIP,
									Order: ElevOrder{
										Status:     Awaiting,
										AssignedTo: msg.AssignedTo,
										Timer:      externalOrderMatrix[msg.Floor][msg.ButtonType].Timer,
									},
								}
							})
							sendOrderChannel <- ElevOrderMessage{
								Floor:      msg.Floor,
								ButtonType: msg.ButtonType,
								AssignedTo: msg.AssignedTo,
								OriginIP:   msg.OriginIP,
								SenderIP:   localIP,
								Event:      EvOrderConfirmed,
							}
						} else {
							printDebug("Received an EvAckNewOrder on an order that is Awaiting")
						}
					case UnderExecution:
						printDebug("Received an EvAckNewOrder on an order witch is UnderExecution")
					case NotActive:
						printDebug("Received an EvAckNewOrder on an order witch is NotActive")
					}
				}

			case EvOrderConfirmed:
				switch externalOrderMatrix[msg.Floor][msg.ButtonType].Status {
				case NotActive:
					printDebug("Recived an EvOrderConfirmed on an order who is not active.")
					if msg.SenderIP != localIP {
						printDebug("Adding it to list if it is not assigned to me.")
						if msg.AssignedTo != localIP {
							externalOrderMatrix[msg.Floor][msg.ButtonType].Status = UnderExecution
							externalOrderMatrix[msg.Floor][msg.ButtonType].AssignedTo = msg.AssignedTo
							externalOrderMatrix[msg.Floor][msg.ButtonType].DeleteConfirmedBy()
						}
					}
				case Awaiting:
					printDebug("Sending EvAckOrderConfirmed on " + ButtonType[msg.ButtonType] + " on floor " + strconv.Itoa(msg.Floor) + " assigned to " + msg.AssignedTo)
					sendOrderChannel <- ElevOrderMessage{
						Floor:      msg.Floor,
						ButtonType: msg.ButtonType,
						AssignedTo: msg.AssignedTo,
						OriginIP:   msg.OriginIP,
						SenderIP:   localIP,
						Event:      EvAckOrderConfirmed,
					}
					externalOrderMatrix[msg.Floor][msg.ButtonType].Status = UnderExecution
					if msg.AssignedTo == localIP {
						if knownElevators[localIP].IsIdle() && knownElevators[localIP].State.LastFloor == msg.Floor {
							externalOrderMatrix[msg.Floor][msg.ButtonType].Status = NotActive
							lightChannel <- elev.ElevLight{Type: INDICATOR_DOOR, Active: true}
							doorTimer.Reset(doorWaitTime)
							knownElevators[localIP].State.DoorIsOpen = true
							sendRestoreChannel <- ResolveBackupState(knownElevators[localIP], externalOrderMatrix)
							sendOrderChannel <- ElevOrderMessage{
								Floor:      msg.Floor,
								ButtonType: msg.ButtonType,
								AssignedTo: msg.AssignedTo,
								OriginIP:   msg.OriginIP,
								SenderIP:   localIP,
								Event:      EvOrderDone,
							}
						} else if !knownElevators[localIP].State.IsMoving && knownElevators[localIP].State.LastFloor == msg.Floor &&
							(knownElevators[localIP].ResolveExtendedElevState(externalOrderMatrix).GetNextDirection() == STOP ||
								knownElevators[localIP].ResolveExtendedElevState(externalOrderMatrix).GetNextButtonDirection() == msg.ButtonType) {
							printDebug("Reset doorTimer")
							sendRestoreChannel <- ResolveBackupState(knownElevators[localIP], externalOrderMatrix)
							doorTimer.Reset(doorWaitTime)
							knownElevators[localIP].State.DoorIsOpen = true
							printDebug("Sending order done on " + ButtonType[msg.ButtonType] + " on floor " + strconv.Itoa(msg.Floor))
							externalOrderMatrix[msg.Floor][msg.ButtonType].Status = NotActive
							sendOrderChannel <- ElevOrderMessage{
								Floor:      msg.Floor,
								ButtonType: msg.ButtonType,
								AssignedTo: msg.AssignedTo,
								OriginIP:   msg.OriginIP,
								SenderIP:   localIP,
								Event:      EvOrderDone,
							}
						} else if knownElevators[localIP].IsIdle() && !knownElevators[localIP].State.DoorIsOpen {
							lightChannel <- elev.ElevLight{Type: msg.ButtonType, Floor: msg.Floor, Active: true}
							doorTimer.Reset(0 * time.Second)
						} else {
							lightChannel <- elev.ElevLight{Type: msg.ButtonType, Floor: msg.Floor, Active: true}
						}
					} else {
						lightChannel <- elev.ElevLight{Type: msg.ButtonType, Floor: msg.Floor, Active: true}
					}
					if msg.OriginIP != localIP {
						printDebug("Starting timeoutTimer [Excecution timeout] on order " + ButtonType[msg.ButtonType] + " on floor " + strconv.Itoa(msg.Floor))
						timeout := orderTimeout
						if msg.AssignedTo != localIP {
							timeout = 2 * orderTimeout
						}
						externalOrderMatrix[msg.Floor][msg.ButtonType].Timer = time.AfterFunc(timeout, func() {
							log.Println("TIMEOUT:\t An order under execution timed out.")
							timeoutChannel <- ExtendedElevOrder{
								Floor:    msg.Floor,
								Type:     msg.ButtonType,
								OriginIP: msg.OriginIP,
								Order:    externalOrderMatrix[msg.Floor][msg.ButtonType],
							}
						})
					}

				case UnderExecution:
					sendOrderChannel <- ElevOrderMessage{
						Floor:      msg.Floor,
						ButtonType: msg.ButtonType,
						AssignedTo: msg.AssignedTo,
						OriginIP:   msg.OriginIP,
						SenderIP:   localIP,
						Event:      EvAckOrderConfirmed,
					}
					if externalOrderMatrix[msg.Floor][msg.ButtonType].AssignedTo != msg.AssignedTo {
						log.Println("MAIN:\t Received an EvOrderConfirmed on an order witch is UnderExecution by another elevator!")
						log.Println("          \t The order %v on floor %v was AssignedTo %v and %v had assigned it to %v\n",
							ButtonType[msg.ButtonType], msg.Floor, externalOrderMatrix[msg.Floor][msg.ButtonType].AssignedTo, msg.SenderIP, msg.AssignedTo)
					}
				}
			case EvAckOrderConfirmed:
				if msg.OriginIP == localIP {
					printDebug("EvAckOrderConfirmed: " + ButtonType[msg.ButtonType] + " on floor " + strconv.Itoa(msg.Floor) + " who is " + ElevOrderStatus[externalOrderMatrix[msg.Floor][msg.ButtonType].Status])
					switch externalOrderMatrix[msg.Floor][msg.ButtonType].Status {
					case NotActive:
						printDebug("EvAckOrderConfirmed while NotActive")
					case Awaiting:
						printDebug("EvAckOrderConfirmed while Awaiting")
					case UnderExecution:
						externalOrderMatrix[msg.Floor][msg.ButtonType].ConfirmedBy[msg.SenderIP] = true
						if allActiveElevatorsHaveAcked(externalOrderMatrix, activeElevators, msg) {
							log.Println("MAIN:\t Recived AckOrderConfirmed from all active elevators.")
							printDebug("Stoping timeoutTimer [EvAckOrderConfirmed] on order " + ButtonType[msg.ButtonType] + " on floor " + strconv.Itoa(msg.Floor))
							externalOrderMatrix[msg.Floor][msg.ButtonType].StopTimer()
							externalOrderMatrix[msg.Floor][msg.ButtonType].DeleteConfirmedBy()
							printDebug("Starting timeoutTimer [Excecution timeout] on order " + ButtonType[msg.ButtonType] + " on floor" + strconv.Itoa(msg.Floor))
							timeout := orderTimeout
							if msg.AssignedTo != localIP {
								timeout = 2 * orderTimeout
							}
							externalOrderMatrix[msg.Floor][msg.ButtonType].Timer = time.AfterFunc(timeout, func() {
								log.Println("TIMEOUT:\t An order under execution timed out.")
								timeoutChannel <- ExtendedElevOrder{
									Floor:    msg.Floor,
									Type:     msg.ButtonType,
									OriginIP: msg.OriginIP,
									Order:    externalOrderMatrix[msg.Floor][msg.ButtonType],
								}
							})
						}
					}
				}
			case EvOrderDone:
				log.Println("MAIN:\t " + msg.AssignedTo + " is done with order " + ButtonType[msg.ButtonType] + " on floor " + strconv.Itoa(msg.Floor))
				externalOrderMatrix[msg.Floor][msg.ButtonType].Status = NotActive
				externalOrderMatrix[msg.Floor][msg.ButtonType].AssignedTo = ""
				externalOrderMatrix[msg.Floor][msg.ButtonType].DeleteConfirmedBy()
				printDebug("Stoping timeoutTimer [Execution timeout] on order " + ButtonType[msg.ButtonType] + " on floor " + strconv.Itoa(msg.Floor))
				externalOrderMatrix[msg.Floor][msg.ButtonType].StopTimer()
				lightChannel <- elev.ElevLight{Floor: msg.Floor, Type: msg.ButtonType, Active: false}
				sendOrderChannel <- ElevOrderMessage{
					Floor:      msg.Floor,
					ButtonType: msg.ButtonType,
					AssignedTo: msg.AssignedTo,
					OriginIP:   msg.OriginIP,
					SenderIP:   localIP,
					Event:      EvAckOrderDone,
				}
				if msg.AssignedTo == localIP {
					externalOrderMatrix[msg.Floor][msg.ButtonType].Timer = time.AfterFunc(ackTimeout, func() {
						log.Println("TIMEOUT:\t An orderDone was not ack´d by all activeElevators. Resending...")
						sendOrderChannel <- ElevOrderMessage{
							Floor:      msg.Floor,
							ButtonType: msg.ButtonType,
							AssignedTo: msg.AssignedTo,
							OriginIP:   msg.OriginIP,
							SenderIP:   localIP,
							Event:      EvAckOrderDone,
						}
					})
				}

			case EvAckOrderDone:
				printDebug("Received an EvAckOrderDone from " + msg.SenderIP)
				if msg.AssignedTo == localIP {
					externalOrderMatrix[msg.Floor][msg.ButtonType].ConfirmedBy[msg.SenderIP] = true
					if allActiveElevatorsHaveAcked(externalOrderMatrix, activeElevators, msg) {
						log.Println("MAIN:\t Recived AckOrderDone from all active elevators.")
						printDebug("Stoping timeoutTimer [EvAckOrderDone] on order " + ButtonType[msg.ButtonType] + " on floor " + strconv.Itoa(msg.Floor))
						externalOrderMatrix[msg.Floor][msg.ButtonType].StopTimer()
						externalOrderMatrix[msg.Floor][msg.ButtonType].DeleteConfirmedBy()
					}
				}

			case EvReassignOrder:
				switch externalOrderMatrix[msg.Floor][msg.ButtonType].Status {
				case NotActive:
					printDebug("Received an EvReassignOrder on an order that is NotActive")
				case Awaiting:
					printDebug("Received an EvReassignOrder on an order that is Awaiting")
				case UnderExecution:
					printDebug("Received an EvReassignOrder on an order that is UnderExecution")
					externalOrderMatrix[msg.Floor][msg.ButtonType].StopTimer()
					externalOrderMatrix[msg.Floor][msg.ButtonType].Status = NotActive
					externalOrderMatrix[msg.Floor][msg.ButtonType].DeleteConfirmedBy()
					receiveOrderChannel <- ElevOrderMessage{
						Floor:      msg.Floor,
						ButtonType: msg.ButtonType,
						AssignedTo: msg.AssignedTo,
						OriginIP:   msg.OriginIP,
						SenderIP:   msg.SenderIP,
						Event:      EvNewOrder,
					}
				}
			default:
				printDebug("Recived an invalid ElevOrderMessage from " + msg.SenderIP)
				externalOrderMatrix[msg.Floor][msg.ButtonType].DeleteConfirmedBy()
			}

		//---------------TIMEOUT HANDLER------------------------------
		case msg := <-timeoutChannel:
			log.Println("MAIN:\t TimeoutTimer timed out on order", ButtonType[msg.Type], "on floor", strconv.Itoa(msg.Floor), "assigned to", msg.Order.AssignedTo)
			switch msg.Order.Status {
			case NotActive: //EvAckNewOrder failed
				log.Println("MAIN:\t Not all elevators Ack'd newOrder. Resending")
				sendOrderChannel <- ElevOrderMessage{
					Floor:      msg.Floor,
					ButtonType: msg.Type,
					AssignedTo: msg.Order.AssignedTo,
					OriginIP:   localIP,
					SenderIP:   localIP,
					Event:      EvNewOrder,
				}
			case Awaiting: //EvAckOrderConfirmed failed
				log.Println("MAIN:\t Not all elevators Ack'd OrderConfirmed. Resending")
				sendOrderChannel <- ElevOrderMessage{
					Floor:      msg.Floor,
					ButtonType: msg.Type,
					AssignedTo: msg.Order.AssignedTo,
					OriginIP:   msg.OriginIP,
					SenderIP:   localIP,
					Event:      EvOrderConfirmed,
				}

			case UnderExecution:
				if msg.Order.AssignedTo == localIP { //Something is blocking the elevator from finishing the order -> I have failed [ I can not go on! :( ]
					motorChannel <- STOP
					time.Sleep(100 * time.Millisecond)
					log.Fatal("MAIN:\t An order under excecution timed out. I´m out!")
				}
				//Somebody else have to take the order... The first elevator to timeout will be new OriginIP
				log.Println("MAIN:\t An order has not been done... Somebody else need to take it.")
				assignedIP, err := cost.AssignNewOrder(knownElevators, activeElevators, externalOrderMatrix, msg.Floor, msg.Type)
				if err != nil {
					log.Fatal(err)
				}
				sendOrderChannel <- ElevOrderMessage{
					Floor:      msg.Floor,
					ButtonType: msg.Type,
					AssignedTo: assignedIP,
					OriginIP:   localIP,
					SenderIP:   localIP,
					Event:      EvReassignOrder,
				}
			default:
				printDebug("Recived an invalid ExtendedElevOrderMessage in TimeoutHandler")
			}

		//-------HARDWARE-------
		case button := <-buttonChannel:
			log.Println("MAIN:\t Received a", ButtonType[button.Type], "from floor", button.Floor, ".Number of activeElevators", len(activeElevators))
			switch button.Type {
			case BUTTON_CALL_UP, BUTTON_CALL_DOWN:
				if _, ok := activeElevators[localIP]; !ok {
					log.Println("MAIN:\t Can not accept new external order while offline!")
				} else {
					if assignedIP, err := cost.AssignNewOrder(knownElevators, activeElevators, externalOrderMatrix, button.Floor, button.Type); err != nil {
						log.Fatal(err)
					} else {
						sendOrderChannel <- ElevOrderMessage{
							Floor:      button.Floor,
							ButtonType: button.Type,
							AssignedTo: assignedIP,
							OriginIP:   localIP,
							SenderIP:   localIP,
							Event:      EvNewOrder,
						}
					}
				}
			case BUTTON_COMMAND:
				if !knownElevators[localIP].State.IsMoving && knownElevators[localIP].State.LastFloor == button.Floor {
					lightChannel <- elev.ElevLight{Type: INDICATOR_DOOR, Active: true}
					log.Println("MAIN:\t Opening doors")
					doorTimer.Reset(doorWaitTime)
					knownElevators[localIP].State.DoorIsOpen = true
					sendRestoreChannel <- ResolveBackupState(knownElevators[localIP], externalOrderMatrix)
				} else {
					printDebug("Added internal order to queue")
					knownElevators[localIP].SetInternalOrder(button.Floor)
					sendRestoreChannel <- ResolveBackupState(knownElevators[localIP], externalOrderMatrix)
					lightChannel <- elev.ElevLight{Type: button.Type, Floor: button.Floor, Active: true}
					if knownElevators[localIP].IsIdle() && !knownElevators[localIP].State.DoorIsOpen {
						doorTimer.Reset(0 * time.Millisecond)
					}
				}

			case BUTTON_STOP:
				motorChannel <- STOP
				lightChannel <- elev.ElevLight{Type: BUTTON_STOP, Active: true}
				fmt.Println("\n---------------------         SOMEBODY KILLED THIS ELEVATOR!     ---------------------")
				time.Sleep(200 * time.Millisecond)
				os.Exit(1)
			default:
				printDebug("Recived an ButtonType from the elev driver")
			}

		case floor := <-floorChannel:
			log.Println("MAIN:\t evFloorReached: ", floor)
			knownElevators[localIP].SetLastFloor(floor)
			if knownElevators[localIP].ResolveExtendedElevState(externalOrderMatrix).ShouldStop() {
				motorChannel <- STOP
				knownElevators[localIP].SetMoving(false)
				log.Println("MAIN:\t Opening doors")
				doorTimer.Reset(doorWaitTime)
				lightChannel <- elev.ElevLight{Type: INDICATOR_DOOR, Active: true}
				knownElevators[localIP].ClearInternalOrderAtCurrentFloor()
				lightChannel <- elev.ElevLight{Floor: floor, Type: BUTTON_COMMAND, Active: false}
				orders := knownElevators[localIP].ResolveExtendedElevState(externalOrderMatrix).FindExternalOrdersAtCurrentFloor()
				for _, o := range orders {
					externalOrderMatrix[o.Floor][o.Type].Status = NotActive
					externalOrderMatrix[o.Floor][o.Type].AssignedTo = ""
					externalOrderMatrix[o.Floor][o.Type].DeleteConfirmedBy()
					printDebug("Stoping timeoutTimer [Execution timeout] on order " + ButtonType[o.Type] + " on floor " + strconv.Itoa(o.Floor))
					externalOrderMatrix[o.Floor][o.Type].StopTimer()
					lightChannel <- elev.ElevLight{Floor: o.Floor, Type: o.Type, Active: false}
					externalOrderMatrix[o.Floor][o.Type].Timer = time.AfterFunc(ackTimeout, func() {
						log.Println("TIMEOUT:\t An orderDone was not ack´d by all activeElevators. Resending...")
						sendOrderChannel <- ElevOrderMessage{
							Floor:      o.Floor,
							ButtonType: o.Type,
							AssignedTo: o.Order.AssignedTo,
							OriginIP:   o.OriginIP,
							SenderIP:   localIP,
							Event:      EvOrderDone,
						}
					})
					printDebug("Sending orderDoneMessage on " + ButtonType[o.Type] + " on floor " + strconv.Itoa(o.Floor))
					sendOrderChannel <- ElevOrderMessage{
						Floor:      o.Floor,
						ButtonType: o.Type,
						AssignedTo: o.Order.AssignedTo,
						OriginIP:   o.OriginIP,
						SenderIP:   localIP,
						Event:      EvOrderDone,
					}
				}
			}
			sendRestoreChannel <- ResolveBackupState(knownElevators[localIP], externalOrderMatrix)

		//-------TIMERS-------
		case <-iAmAliveTick.C:
			sendRestoreChannel <- ResolveIAmAliveMessage(knownElevators[localIP])

		case <-checkAliveTick.C:
			updateActiveElevators(knownElevators, activeElevators, localIP, iAmAliveLimit)

		case <-doorTimer.C:
			printDebug("evDoorTimeout")
			log.Println("MAIN:\t Closing doors")
			knownElevators[localIP].State.DoorIsOpen = false
			lightChannel <- elev.ElevLight{Type: INDICATOR_DOOR, Active: false}
			if knownElevators[localIP].ResolveExtendedElevState(externalOrderMatrix).HaveOrders() {
				knownElevators[localIP].SetDirection(knownElevators[localIP].ResolveExtendedElevState(externalOrderMatrix).GetNextDirection())
				knownElevators[localIP].SetMoving(knownElevators[localIP].State.Direction != STOP)
				log.Println("MAIN:\t I have orders to do...")
				log.Println("MAIN:\t Going direction", MotorCommands[knownElevators[localIP].State.Direction+1])
				lightChannel <- elev.ElevLight{Floor: knownElevators[localIP].State.LastFloor, Type: BUTTON_COMMAND, Active: false}
				motorChannel <- knownElevators[localIP].State.Direction
			} else {
				log.Println("MAIN:\t I dont have any order to do")
				knownElevators[localIP].SetMoving(false)
				knownElevators[localIP].SetDirection(STOP)
			}
			sendRestoreChannel <- ResolveBackupState(knownElevators[localIP], externalOrderMatrix)
		}
	}
}

//------------------SUPPORT FUNCTIONS-------------
func sendOrderDoneMessages(orders []ExtendedElevOrder, sendOrderChannel chan<- ElevOrderMessage, localIP string) {
	for _, order := range orders {
		printDebug("Sending orderDoneMessage on " + ButtonType[order.Type] + " on floor " + strconv.Itoa(order.Floor))
		sendOrderChannel <- ElevOrderMessage{
			Floor:      order.Floor,
			ButtonType: order.Type,
			AssignedTo: order.Order.AssignedTo,
			OriginIP:   order.OriginIP,
			SenderIP:   localIP,
			Event:      EvOrderDone,
		}
	}
}

func allActiveElevatorsHaveAcked(externalOrderMatrix [N_FLOORS][2]ElevOrder, activeElevators map[string]bool, msg ElevOrderMessage) bool {
	for elevator, _ := range activeElevators {
		if _, confirmed := externalOrderMatrix[msg.Floor][msg.ButtonType].ConfirmedBy[elevator]; !confirmed {
			return false
		}
	}
	return true
}

func initNetwork(connectionAttempsLimit int, receiveOrderChannel, sendOrderChannel chan ElevOrderMessage, receiveRestoreChannel, sendRestoreChannel chan ElevRestoreMessage) (localIP string, err error) {
	for i := 0; i <= connectionAttempsLimit; i++ {
		localIP, err := network.Init(receiveOrderChannel, sendOrderChannel, receiveRestoreChannel, sendRestoreChannel)
		if err != nil {
			if i == 0 {
				log.Println("MAIN:\t Network init was not successfull. Trying some more times")
			} else if i == connectionAttempsLimit {
				return "", err
			}
			time.Sleep(3 * time.Second)
		} else {
			return localIP, nil
		}
	}
	return "", nil
}

func updateActiveElevators(knownElevators map[string]*Elevator, activeElevators map[string]bool, localIP string, iAmAliveLimit time.Duration) {
	for key := range knownElevators {
		if time.Since(knownElevators[key].Time) > iAmAliveLimit {
			if activeElevators[key] == true {
				log.Printf("MAIN:\t Removed elevator %s in activeElevators\n", knownElevators[key].State.LocalIP)
				delete(activeElevators, key)
			}
		} else {
			if activeElevators[key] != true {
				activeElevators[key] = true
				log.Printf("MAIN:\t Added elevator %s in activeElevators\n", knownElevators[key].State.LocalIP)
			}
		}
	}
}

func printDebug(s string) {
	if debug {
		log.Println("MAIN:\t", s)
	}
}
