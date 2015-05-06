package network

import (
	. "../typedef"
	"../udp"
	"encoding/json"
	"log"
)

const debug = false

func Init(reciveOrderChannel chan<- ElevOrderMessage,
	sendOrderChannel <-chan ElevOrderMessage,
	reciveRestoreChannel chan<- ElevRestoreMessage,
	sendRestoreChannel <-chan ElevRestoreMessage) (localIP string, err error) {
	const messageSize = 4 * 1024
	const UDPLocalListenPort = 22301
	const UDPBroadcastListenPort = 22302
	UDPSendChannel := make(chan udp.UDPMessage, 10)
	UDPReceiveChannel := make(chan udp.UDPMessage)
	localIP, err = udp.Init(UDPLocalListenPort, UDPBroadcastListenPort, messageSize, UDPSendChannel, UDPReceiveChannel)
	if err != nil {
		return "", err
	}
	go reciveMessageHandler(reciveOrderChannel, reciveRestoreChannel, UDPReceiveChannel)
	go sendMessageHandler(sendOrderChannel, sendRestoreChannel, UDPSendChannel)
	return localIP, nil
}

func reciveMessageHandler(reciveOrderChannel chan<- ElevOrderMessage, reciveRestoreChannel chan<- ElevRestoreMessage, UDPReceiveChannel <-chan udp.UDPMessage) {
	for {
		select {
		case msg := <-UDPReceiveChannel:
			var f interface{}
			err := json.Unmarshal(msg.Data[:msg.Length], &f)
			if err != nil {
				printDebug("Error with Unmarshaling a message.")
				log.Println(err)
			} else {
				m := f.(map[string]interface{})
				event := int(m["Event"].(float64))
				if event <= 3 && event >= 0 {
					var restore = ElevRestoreMessage{}
					if err := json.Unmarshal(msg.Data[:msg.Length], &restore); err == nil {
						if restore.IsValid() {
							reciveRestoreChannel <- restore
							printDebug("Recived an ElevRestoreMessage with Event " + EventType[restore.Event])
						} else {
							printDebug("Rejected an ElevRestoreMessage with Event " + EventType[restore.Event])
						}
					} else {
						printDebug("Error with Unmarshaling a ElevStateMessage")
					}
				} else if event >= 4 && event <= 10 {
					var order = ElevOrderMessage{}
					if err := json.Unmarshal(msg.Data[:msg.Length], &order); err == nil {
						if order.IsValid() {
							reciveOrderChannel <- order
							printDebug("Recived an ElevOrderMessage with Event " + EventType[order.Event])
						} else {
							printDebug("Rejected an ElevOrderMessage with Event " + EventType[order.Event])
						}
					}
				} else {
					printDebug("Recived an unknown message type")
				}
			}
		}
	}
}

func sendMessageHandler(sendOrderChannel <-chan ElevOrderMessage, sendRestoreChannel <-chan ElevRestoreMessage, UDPSendChannel chan<- udp.UDPMessage) {
	for {
		select {
		case msg := <-sendOrderChannel:
			networkPack, err := json.Marshal(msg)
			if err != nil {
				printDebug("Error Marshalling an outgoing message")
				log.Println(err)
			} else {
				UDPSendChannel <- udp.UDPMessage{Raddr: "broadcast", Data: networkPack}
				printDebug("Sent an ElevOrderMessage with Event " + EventType[msg.Event])
			}

		case msg := <-sendRestoreChannel:
			networkPack, err := json.Marshal(msg)
			if err != nil {
				printDebug("Error Marshalling an outgoing message")
				log.Println(err)
			} else {
				UDPSendChannel <- udp.UDPMessage{Raddr: "broadcast", Data: networkPack}
				printDebug("Sent an ElevOrderMessage with Event " + EventType[msg.Event])
			}
		}
	}
}

func printDebug(s string) {
	if debug {
		log.Println("NETWORK:\t", s)
	}
}
