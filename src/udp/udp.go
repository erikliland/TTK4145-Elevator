package udp

import (
	"log"
	"net"
	"strconv"
)

const debug = false

var laddr *net.UDPAddr //Local address
var baddr *net.UDPAddr //Broadcast address

type UDPMessage struct {
	Raddr  string //"broadcast" or "xxx.xxx.xxx.xxx:yyyy"
	Data   []byte
	Length int //length of received data, in #bytes // N/A for sending
}

func Init(localListenPort, broadcastListenPort, messageSize int, sendChannel <-chan UDPMessage, receiveChannel chan<- UDPMessage) (localIP string, err error) {
	//Generating broadcast address
	baddr, err = net.ResolveUDPAddr("udp4", "255.255.255.255:"+strconv.Itoa(broadcastListenPort))
	if err != nil {
		log.Println("UDP:\t Could not resolve UDPAddr")
		return "", err
	} else if debug {
		log.Printf("UDP:\t Generating broadcast address:\t%s \n", baddr.String())
	}

	//Generating localaddress
	tempConn, err := net.DialUDP("udp4", nil, baddr)
	if err != nil {
		log.Println("UDP:\t It looks like you don´t have a network connection")
		return "", err
	} else {
		defer tempConn.Close()
	}
	tempAddr := tempConn.LocalAddr()
	laddr, err = net.ResolveUDPAddr("udp4", tempAddr.String())
	if err != nil {
		log.Println("UDP:\t Could not resolve local adress")
		return "", err
	} else if debug {
		log.Printf("UDP:\t Generating local address:\t%s \n", laddr.String())
	}
	laddr.Port = localListenPort

	//Creating local listening connections
	localListenConn, err := net.ListenUDP("udp4", laddr)
	if err != nil {
		log.Println("UDP:\t Could not create a UDP listener socket")
		return "", err
	} else if debug {
		log.Println("UDP:\t Created a UDP listener socket")
	}

	//Creating listener on broadcast connection
	broadcastListenConn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(0, 0, 0, 0), Port: broadcastListenPort})
	if err != nil {
		log.Println("UDP:\t Could not create a UDP broadcastListen socket")
		localListenConn.Close()
		return "", err
	} else if debug {
		log.Println("UDP:\t Created a UDP broadcastListen socket")
	}
	go udpReciveServer(localListenConn, broadcastListenConn, messageSize, receiveChannel)
	go udpTransmittServer(localListenConn, broadcastListenConn, localListenPort, broadcastListenPort, sendChannel)
	return laddr.IP.String(), err
}

func udpTransmittServer(lconn, bconn *net.UDPConn, localListenPort, broadcastListenPort int, sendChannel <-chan UDPMessage) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("UDPConnectionReader:\t Error in UDPTransmitServer: %s \n Closing connection.", r)
			lconn.Close()
			bconn.Close()
		}
	}()
	for {
		if debug {
			log.Println("UDPTransmitServer:\t Waiting on new value on sendChannel")
		}
		select {
		case msg := <-sendChannel:
			if debug {
				log.Println("UDPTransmitServer:\t Start sending an ElevState package to: ", msg.Raddr)
				log.Println("UDP-Send:\t", string(msg.Data))
			}
			if msg.Raddr == "broadcast" {
				n, err := lconn.WriteToUDP(msg.Data, baddr)
				if (err != nil || n < 0) && debug {
					log.Printf("UDPTransmitServer:\t Error ending broadcast message\n")
					log.Println(err)
				}
			} else {
				raddr, err := net.ResolveUDPAddr("udp", msg.Raddr+":"+strconv.Itoa(localListenPort))
				if err != nil {
					log.Printf("UDPTransmitServer:\t Could not resolve raddr\n")
					log.Fatal(err)
				}
				if n, err := lconn.WriteToUDP(msg.Data, raddr); err != nil || n < 0 {
					log.Println("UDPTransmitServer:\t Error: Sending p2p message")
					log.Println(err)
				}
			}
		}
	}
}

func udpReciveServer(lconn, bconn *net.UDPConn, messageSize int, receiveChannel chan<- UDPMessage) {
	defer func() {
		if r := recover(); r != nil {
			log.Println("UDP:\t ERROR in UDPReciveServer: %s \n Closing connection.", r)
			lconn.Close()
			bconn.Close()
		}
	}()
	bconn_rcv_ch := make(chan UDPMessage)
	lconn_rcv_ch := make(chan UDPMessage)
	go udpConnectionReader(lconn, messageSize, lconn_rcv_ch)
	go udpConnectionReader(bconn, messageSize, bconn_rcv_ch)
	for {
		select {
		case msg := <-bconn_rcv_ch:
			receiveChannel <- msg
		case msg := <-lconn_rcv_ch:
			receiveChannel <- msg
		}
	}
}

func udpConnectionReader(conn *net.UDPConn, messageSize int, rcv_ch chan<- UDPMessage) {
	defer func() {
		if r := recover(); r != nil {
			log.Println("UDPConnectionReader:\t ERROR in udpConnectionReader:\t %s \n Closing connection.", r)
			conn.Close()
		}
	}()

	for {
		if debug {
			log.Printf("UDPConnectionReader:\t Waiting on data from UDPConn %s\n", conn.LocalAddr().String())
		}
		buf := make([]byte, messageSize) //TODO: Should be done without allocating new memory every time
		n, raddr, err := conn.ReadFromUDP(buf)
		if err != nil || n < 0 || n > messageSize {
			log.Println("UDPConnectionReader:\t Error in ReadFromUDP:", err)
		} else {
			if debug {
				log.Println("UDPConnectionReader:\t Received package from:", raddr.String())
				log.Println("UDP-Listen:\t", string(buf[:]))
			}
			rcv_ch <- UDPMessage{Raddr: raddr.String(), Data: buf[:n], Length: n}
		}
	}
}
