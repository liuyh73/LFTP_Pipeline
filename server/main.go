package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/liuyh73/LFTP_Pipeline/LFTP/models"
)

const (
	server_ip       = "127.0.0.1"
	server_port     = "8808"
	server_send_len = 1982
	server_recv_len = 2000
)

func checkErr(err error) {
	if err != nil {
		log.Println(err)
	}
}

func main() {
	serverAddr := server_ip + ":" + server_port
	serverUDPAddr, err := net.ResolveUDPAddr("udp", serverAddr)
	checkErr(err)

	serverSocket, err := net.ListenUDP("udp", serverUDPAddr)
	checkErr(err)
	defer serverSocket.Close()

	for {
		buf := make([]byte, server_recv_len)
		_, clientUDPAddr, err := serverSocket.ReadFromUDP(buf)

		checkErr(err)
		packet := &models.Packet{}
		packet.FromBytes(buf)
		fmt.Println(packet)
		dataStr := string(packet.Data)
		fmt.Println("Received:", dataStr)
		if strings.Split(dataStr, ": ")[0] == "conn" {
			handleConn(serverSocket, clientUDPAddr)
		} else if strings.Split(dataStr, ": ")[0] == "lget" {
			handleGetFile(serverSocket, clientUDPAddr, packet.Rwnd, strings.Split(dataStr, ": ")[1])
		} else if strings.Split(dataStr, ": ")[0] == "lsend" {
			handlePutFile(serverSocket, clientUDPAddr)
		} else if strings.Split(dataStr, ": ")[0] == "list" {
			handleList(serverSocket, clientUDPAddr)
		}
	}
}

func handleConn(serverSocket *net.UDPConn, clientUDPAddr *net.UDPAddr) {
	// packet := models.NewPacket(byte(0), byte(0), byte(0), byte(0), []byte("Connected!"))
	// _, err := serverSocket.WriteToUDP(packet.ToBytes(), clientUDPAddr)
	// checkErr(err)
	// fmt.Println("Connected to " + clientUDPAddr.String())
}

func handleGetFile(serverSocket *net.UDPConn, clientUDPAddr *net.UDPAddr, Rwnd rune, pathname string) {
	_, err := os.Stat(pathname)
	// serverSocket.SetDeadline(time.Now().Add(10 * time.Second))
	// lget file不存在
	if os.IsNotExist(err) {
		fmt.Printf("The file %s doesn't exist", pathname)
		packetSnd := models.NewPacket(rune(0), rune(0), rune(0), byte(0), byte(0), []byte(fmt.Sprintf("The file %s doesn't exist", pathname)))
		serverSocket.WriteToUDP(packetSnd.ToBytes(), clientUDPAddr)
		return
	}
	// 打开该文件
	file, err := os.Open(pathname)
	defer file.Close()
	if err != nil {
		fmt.Fprintf(os.Stderr, "An error occurred on opening the inputfile: %s\nDoes the file exist?\n", pathname)
		return
	}
	// 设置base、nextseqnum
	base := rune(1)
	nextseqnum := rune(1)
	// 设置滑动窗口宽度
	rwnd := Rwnd
	// 设置定时器
	timer := time.NewTimer(5 * time.Second)
	// 缓存当前串口已发送但是未确认的包
	packets := make([]*models.Packet, 0)
	// 协程同步
	var wg sync.WaitGroup
	// 设置stopTimer结束定时器信号
	stopTimer := make(chan int)
	// 设置stopRcv结束接收数据包信号
	stopRcv := make(chan int)
	// 文件读取结束标志
	finished := 0

	// sync.WaitGroup Add添加两个协程
	wg.Add(2)
	// 开一个协程后台接收客户端发送回来的确认包
	go func() {
		defer wg.Done()
		for {
			rcvpkt := &models.Packet{}
			rcvpkt.FromBytes(rdt_rcv(serverSocket))
			fmt.Println("rcvpkt.Ack: " + strconv.Itoa(int(rcvpkt.Ack)))
			rwnd = rcvpkt.Rwnd
			if base > rcvpkt.Ack {
				return
			}
			if rcvpkt.Ack == packets[0].Seqnum {
				packets = packets[1:]
			}
			base = rcvpkt.Ack + 1
			if base == nextseqnum {
				// 停止计时器stop
				timer.Stop()
			} else {
				// 启动定时器reset
				timer.Reset(1 * time.Second)
			}
			if rcvpkt.Finished == byte(1) {
				fmt.Println("rcvpkt.Finished" + strconv.Itoa(int(rcvpkt.Finished)))
				sndpkt := models.NewPacket(rune(nextseqnum), rcvpkt.Seqnum, rune(0), byte(1), byte(finished), []byte{})
				udt_send(serverSocket, sndpkt, clientUDPAddr)
				packets = append(packets, sndpkt)
				nextseqnum += 1
			}
			select {
			case <-stopRcv:
				return
			default:
			}
		}
	}()

	// 开一个协程来进行定时器设置
	// 如果超时，重新发送数据包, 设置定时器
	go func() {
		defer wg.Done()
		for {
			select {
			case <-timer.C:
				timer.Reset(1 * time.Second)
				for i, sndpkt := range packets {
					if i < int(rwnd) {
						fmt.Println("timer.timeout: send pkg" + strconv.Itoa(int(sndpkt.Seqnum)))
						udt_send(serverSocket, sndpkt, clientUDPAddr)
					}
				}
			case <-stopTimer:
				return
			}
		}
	}()

	// 直到数据发送完毕,确认客户端接收到所有数据、退出循环
	for {
		if nextseqnum <= base+rwnd-1 {
			buf := make([]byte, server_send_len)
			_, err := file.Read(buf)
			if err == io.EOF {
				finished = 1
			}
			fmt.Println("main routine: send pkg" + strconv.Itoa(int(nextseqnum)))
			sndpkt := models.NewPacket(rune(nextseqnum), rune(0), rune(0), byte(1), byte(finished), buf)
			udt_send(serverSocket, sndpkt, clientUDPAddr)
			packets = append(packets, sndpkt)
			if base == nextseqnum {
				timer.Reset(1 * time.Second)
			}
			nextseqnum += 1
			if finished == 1 {
				break
			}
		}
	}
	time.Sleep(10 * time.Second)
	stopTimer <- 1
	stopRcv <- 1
	wg.Wait()
	fmt.Println("传输结束")
}

func handlePutFile(serverSocket *net.UDPConn, clientUDPAddr *net.UDPAddr) {

}

func handleList(serverSocket *net.UDPConn, clientUDPAddr *net.UDPAddr) {

}

func udt_send(serverSocket *net.UDPConn, sndpkt *models.Packet, clientUDPAddr *net.UDPAddr) {
	_, err := serverSocket.WriteToUDP(sndpkt.ToBytes(), clientUDPAddr)
	fmt.Println("Write Length:" + strconv.Itoa(int(sndpkt.Length)))
	checkErr(err)
}

func rdt_rcv(serverSocket *net.UDPConn) []byte {
	buf := make([]byte, server_recv_len)
	_, err := serverSocket.Read(buf)
	checkErr(err)
	return buf
}
