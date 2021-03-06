package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/liuyh73/LFTP_Pipeline/LFTP/models"
	"github.com/liuyh73/LFTP_Pipeline/server/log"
)

const (
	server_ip       = "127.0.0.1"
	server_port     = "8808"
	server_send_len = 1982
	server_recv_len = 2000
)

func checkErr(err error) {
	if err != nil {
		log.Logger.Println(err)
	}
}

func main() {
	serverAddr := server_ip + ":" + server_port
	serverUDPAddr, err := net.ResolveUDPAddr("udp", serverAddr)
	checkErr(err)

	var serverSocket *net.UDPConn
	for {
		// 获取监听UDPAddr的套接字
		serverSocket, err = net.ListenUDP("udp", serverUDPAddr)
		checkErr(err)
		// 获取接收方命令包
		buf := make([]byte, server_recv_len)
		_, clientUDPAddr, err := serverSocket.ReadFromUDP(buf)
		checkErr(err)
		// 读取命令包
		cmdRcvpkt := &models.Packet{}
		cmdRcvpkt.FromBytes(buf)
		dataStr := string(cmdRcvpkt.Data)
		if strings.Split(dataStr, ": ")[0] == "conn" {
			handleConn(serverSocket, clientUDPAddr)
		} else if strings.Split(dataStr, ": ")[0] == "lget" {
			lgetFile := strings.Split(dataStr, ": ")[1]
			log.Logger.SetPrefix("[LFTP lget " + lgetFile + "]")
			handleGetFile(serverSocket, clientUDPAddr, cmdRcvpkt.Rwnd, lgetFile)
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
		FileNotExists := "文件不存在"
		log.Logger.Println(FileNotExists)
		fmt.Println(FileNotExists)
		packetSnd := models.NewPacket(rune(1), rune(0), rune(0), byte(0), byte(0), rune(len([]byte(FileNotExists))), []byte(FileNotExists))
		serverSocket.WriteToUDP(packetSnd.ToBytes(), clientUDPAddr)
		serverSocket.Close()
		return
	}
	// 打开该文件
	file, err := os.Open(pathname)
	defer file.Close()
	if err != nil {
		OpenFileFailed := "打开文件失败"
		log.Logger.Println(OpenFileFailed)
		fmt.Println(OpenFileFailed)
		packetSnd := models.NewPacket(rune(1), rune(0), rune(0), byte(0), byte(0), rune(len([]byte(OpenFileFailed))), []byte(OpenFileFailed))
		serverSocket.WriteToUDP(packetSnd.ToBytes(), clientUDPAddr)
		serverSocket.Close()
		return
	}
	log.Logger.Println("开始传输...")
	var (
		baseRWMutex       sync.RWMutex
		nextseqnumRWMutex sync.RWMutex
		rwndRwMutex       sync.RWMutex
		packetsMutex      sync.Mutex
	)
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
	go func() {
		for {
			time.Sleep(1 * time.Second)
			fmt.Fprintln(os.Stderr, "111111111111111111;")
		}
	}()
	// sync.WaitGroup Add添加两个协程
	wg.Add(1)
	// 开一个协程后台接收客户端发送回来的确认包
	go func() {
		defer wg.Done()
		for {
			fmt.Fprintln(os.Stderr, "2222222222222222222")
			// select 语句会选择可以读取到值的case继续执行
			// 如果没有default语句，那么直到可以从stopRcv通道读取值之前，该协程都处于阻塞状态。
			select {
			case <-stopRcv:
				log.Logger.Println("退出接收客户端确认包协程")
				return
			default:
			}
			// 接收客户端发送回来的ack确认包
			rcvpkt := &models.Packet{}
			fmt.Println("wait for rcvpkt")
			rcvpkt.FromBytes(rdt_rcv(serverSocket))
			fmt.Fprintln(os.Stderr, "base: ", base, "rcvpkt.Ack: ", rcvpkt.Ack, "nextseqnum: ", nextseqnum, "rwnd: ", rwnd)

			// 获取窗口大小（客户端缓冲空闲大小）
			rwndRwMutex.Lock()
			rwnd = rcvpkt.Rwnd
			rwndRwMutex.Unlock()
			// 如果ack确认包的Ack值大于等于base值，则进入条件句（否则，不进行任何操作）
			if base == rcvpkt.Ack {
				// 如果ack确认包的Ack编号为packets队列中第一个已发送还未确认的包的序号，则弹出该包
				packetsMutex.Lock()
				for i, packet := range packets {
					if rcvpkt.Ack == packet.Seqnum {
						packets = packets[i+1:]
						break
					}
				}
				packetsMutex.Unlock()
				// base值置为rcvpkt.Ack + 1，下一个待确认的包
				baseRWMutex.Lock()
				base = rcvpkt.Ack + 1
				baseRWMutex.Unlock()
				// base序号与nextseqnum相等，表示当前所有包都已确认，没有需要待确认的包
				if base == nextseqnum {
					timer.Stop()
				} else {
					fmt.Println("reset timer")
					if !timer.Stop() && len(timer.C) > 0 {
						<-timer.C
					}
					timer.Reset(1 * time.Second)
				}
				// 如果收到的ack确认包的Finished字段为1，表示文件传输结束（下面会讲到，在客户端发送结束包之前，服务端已经向客户端发送了结束包），等待服务端发送确认包；
				// 此if条件句中发送Finished包
				// 之所以在收到客户端发送的Finished之后并不马上断开连接的原因是：如果服务端不传回一个ack确认包表示已收到客户端Finished信息，则客户端并不知道Finished是否被服务端收到，所以客户端不会轻易断开连接
				// 当服务端发送出ack确认包之后，等待一段时间（此时间要大于定时器的时间），等待客户端会不会再发一个Finished包过来（即服务端的ack包丢失）
				// 如果在这段时间内，客户端不再发送Finished包，则证明客户端成功收到ack确认包，已正常断开连接；之后服务端正常断开连接即可
				// 等待时间在函数最后面设置。
				if rcvpkt.Finished == byte(1) {
					log.Logger.Println("发送Finished数据包")
					sndpkt := models.NewPacket(rune(nextseqnum), rcvpkt.Seqnum, rune(0), byte(1), byte(finished), rune(0), []byte{})
					udt_send(serverSocket, sndpkt, clientUDPAddr)
					packetsMutex.Lock()
					packets = append(packets, sndpkt)
					packetsMutex.Unlock()
					nextseqnumRWMutex.Lock()
					nextseqnum += 1
					nextseqnumRWMutex.Unlock()
				}
			}
		}
	}()

	// 开一个协程来进行定时器设置
	// 如果超时，重新发送数据包, 设置定时器
	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			// 此处select语句与上述协程类似，等待timer定时器超时、结束定时器信号
			fmt.Println("&&&&&&&&&&&&&&&&&&&&&&&&&&&&&")
			select {
			// 超时发生在客户端收不到expectseqnum的包，不断确认已接受的最后一个包的时候；由上面协程可知，当客户端不断确认最后一个已发送的包是，其revpkt.Ack值小于base，不进行任何操作，所以最终会发生超时现象
			case <-timer.C:
				// 如果超时，则我们需要重置定时器
				timer.Reset(1 * time.Second)
				// 发送所有已发送还未确认的包
				fmt.Println("############################")
				baseRWMutex.Lock()
				packetsMutex.Lock()
				base = packets[0].Seqnum
				packetsMutex.Unlock()
				baseRWMutex.Unlock()

				packetsMutex.Lock()
				for i, pkt := range packets {
					log.Logger.Println("数据包", i, len(packets), strconv.Itoa(int(pkt.Seqnum)), "未被接收方确认，再次发送")
					fmt.Println("数据包", i, len(packets), strconv.Itoa(int(pkt.Seqnum)), "未被接收方确认，再次发送")
					udt_send(serverSocket, pkt, clientUDPAddr)
				}
				packetsMutex.Unlock()

			// 收到结束定时的信号，退出协程
			case <-stopTimer:
				log.Logger.Println("退出定时器协程")
				return
			}
			fmt.Println("timer routine")
		}
	}()

	// 主线程循环发送数据包，直到文件内容发送完毕,确认客户端接收到所有数据、退出循环
	for {
		if nextseqnum < base+rwnd {
			buf := make([]byte, server_send_len)
			length, err := file.Read(buf)
			// 读到文件末尾
			if err == io.EOF {
				finished = 1
			}
			// fmt.Println("发送数据包：" + strconv.Itoa(int(nextseqnum)))
			sndpkt := models.NewPacket(rune(nextseqnum), rune(0), rune(0), byte(1), byte(finished), rune(length), buf)
			packetsMutex.Lock()
			packets = append(packets, sndpkt)
			packetsMutex.Unlock()
			udt_send(serverSocket, sndpkt, clientUDPAddr)
			// 如果base == nextseqnum，表示当前链路中没有已发送还未确认的包，也没有定时器，所以我们需要启动定时器
			if base == nextseqnum {
				if !timer.Stop() && len(timer.C) > 0 {
					<-timer.C
				}
				timer.Reset(1 * time.Second)
			}
			// 维护nextsqenum自增
			nextseqnumRWMutex.Lock()
			nextseqnum += 1
			nextseqnumRWMutex.Unlock()
			// 发送文件结束，退出循环
			if finished == 1 {
				break
			}
		}
	}

	// 先定义等待5s，后续过程可能继续修改（5s时间足够服务端重传4-5次最后的Finished ack确认包）；此时间过后，默认双方传输结束
	log.Logger.Println("等待五秒确保客户端成功断开连接之后，服务端断开连接")
	fmt.Println("等待五秒确保客户端成功断开连接之后，服务端断开连接")
	time.Sleep(5 * time.Second)

	// 结束定时器协程
	stopTimer <- 1
	// 关闭连接
	serverSocket.Close()
	// 结束接收ack包协程
	stopRcv <- 1
	// 等待所有协程结束
	wg.Wait()
	fmt.Println("传输结束")
	log.Logger.Println("传输结束")
}

func handlePutFile(serverSocket *net.UDPConn, clientUDPAddr *net.UDPAddr) {

}

func handleList(serverSocket *net.UDPConn, clientUDPAddr *net.UDPAddr) {

}

func udt_send(serverSocket *net.UDPConn, sndpkt *models.Packet, clientUDPAddr *net.UDPAddr) {
	now := time.Now()
	_, err := serverSocket.WriteToUDP(sndpkt.ToBytes(), clientUDPAddr)
	diff := time.Now().Sub(now)
	fmt.Println("diff", diff.Nanoseconds())
	fmt.Println("Write Length:" + strconv.Itoa(int(sndpkt.Length)))
	checkErr(err)
}

func rdt_rcv(serverSocket *net.UDPConn) []byte {
	buf := make([]byte, server_recv_len)
	_, err := serverSocket.Read(buf)
	checkErr(err)
	return buf
}
