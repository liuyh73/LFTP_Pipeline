// Copyright © 2018 NAME HERE <EMAIL ADDRESS>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"fmt"
	"bytes"
	"io"
	"math/rand"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/liuyh73/LFTP_Pipeline/LFTP/config"
	"github.com/liuyh73/LFTP_Pipeline/LFTP/models"
	"github.com/liuyh73/LFTP_Pipeline/LFTP/log"
	"github.com/spf13/cobra"
)

var lgetFile string

// lgetCmd represents the lget command
var lgetCmd = &cobra.Command{
	Use:   "lget",
	Short: "lget command helps us to get a file from server.",
	Long:  `We can use LFTP lget -f <file> to get a file from server.`,
	Run: func(cmd *cobra.Command, args []string) {
		log.Logger.SetPrefix("[LFTP lget " + lgetFile + "]")
		rand.Seed(time.Now().UnixNano())
		stopTimer := make(chan int)
		expectedseqnum := 1
		bufSize := 50
		rwnd := 50
		var bufs bytes.Buffer
		var timer *time.Timer
		var wg sync.WaitGroup
		var sndpkt *models.Packet
		// 获取raddr
		serverAddr := host + ":" + port
		raddr, err := net.ResolveUDPAddr("udp", serverAddr)
		checkErr(err)
		// 获取客户端套接字
		// net.DialUDP("udp", localAddr *UDPAddr, remoteAddr *UDPAddr)
		clientSocket, err := net.DialUDP("udp", nil, raddr)
		checkErr(err)
		defer clientSocket.Close()
		lgetPacket := models.NewPacket(rune(0), rune(0), rune(bufSize), byte(1), byte(0), rune(len([]byte("lget: "+lgetFile))),[]byte("lget: "+lgetFile))

		// 向服务器发送请求
		_, err = clientSocket.Write(lgetPacket.ToBytes())
		checkErr(err)
		// 创建文件句柄
		outputFile, err := os.OpenFile(lgetFile, os.O_CREATE|os.O_TRUNC, 0600)
		checkErr(err)

		// 模拟流控制，开一个协程，每0.5s从buffer中随机读取n个包写入到文件，更新缓冲区大小
		wg.Add(2)
		go func() {
			defer wg.Done()
			for {
				time.Sleep(500 * time.Millisecond)
				// 生成随机数，读取多个包
				count := rand.Intn(bufSize-rwnd+1) + 5
				for i := 0; i < count; i++ {
					// 从buffer中读取长度为config.CLIENT_RECV_KEN长度的包
					buf := make([]byte, config.CLIENT_RECV_LEN)
					length, err := bufs.Read(buf)
					rcvpkt := &models.Packet{}
					rcvpkt.FromBytes(buf)
					// 如果成功读取该数据包（防止buffer为空），则将该包写入到文件
					if length > 0 {
						rwnd += 1
						// 截取buf切片中多余的零
						outputFile.Write(rcvpkt.Data[:rcvpkt.Length])
					}
					// 如果写入的包包含Finished标识，则结束此协程
					if rcvpkt.Finished == byte(1) {
						return
					}
					// 如果buffer读到末尾，则退出内层循环，等待下此读取
					if err == io.EOF {
						break
					}
				}
			}
		}()
		
		// 主线程用于获取数据包并写入到buffer中
		for {
			rcvpkt := &models.Packet{}
			buf := rdt_rcv(clientSocket)
			rcvpkt.FromBytes(buf)
			// 如果接收到的包的序列号，符合想要的包序号，则发送确认包，并将该包写入到buffer，更新rwnd和expectedseqnum
			if rcvpkt.Seqnum == rune(expectedseqnum) && rwnd > 1 {
				rwnd -= 1
				if rcvpkt.Status == byte(0) {
					fmt.Println(string(rcvpkt.Data))
					log.Logger.Println(string(rcvpkt.Data))
					return
				}
				sndpkt = models.NewPacket(rune(expectedseqnum), rune(expectedseqnum), rune(rwnd), byte(1), rcvpkt.Finished, rune(0),[]byte{})

				bufs.Write(buf)
				expectedseqnum += 1
			}
			clientSocket.Write(sndpkt.ToBytes())
			// 如果收到的包包含Finished标识，则接收方也发送包含Finished标识的包，并启动定时器，准备接收服务端Finished确认包
			if rcvpkt.Finished == byte(1) {
				timer = time.NewTimer(1 * time.Second)
				break
			}
		}
		// 如果超时，默认服务端未收到Finished包，重新发送数据包最后的Finished包, 并重置定时器
		go func() {
			defer wg.Done()
			for {
				select {
				case <-timer.C:
					log.Logger.Println("发送Finished数据包" + strconv.Itoa(int(sndpkt.Seqnum)) + "超时")
					clientSocket.Write(sndpkt.ToBytes())
					timer.Reset(1 * time.Second)
				case <-stopTimer:
					return
				}
			}
		}()
		// 等待接收服务端Finished确认包
		for {
			rcvpkt := &models.Packet{}
			rcvpkt.FromBytes(rdt_rcv(clientSocket))
			if rcvpkt.Seqnum == rune(expectedseqnum) && rcvpkt.Finished == 1 {
				log.Logger.Println("成功接收服务端Finished确认包")
				break
			}
		}
		// 结束定时器协程
		stopTimer <- 1
		// 等待所有协程结束
		wg.Wait()
		log.Logger.Println("传输结束")
	},
}

func init() {
	rootCmd.AddCommand(lgetCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// lgetCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// lgetCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	lgetCmd.Flags().StringVarP(&lgetFile, "file", "f", "", "lgetfile filename")
	lgetCmd.Flags().StringVarP(&host, "host", "H", config.SERVER_IP, "Server host")
	lgetCmd.Flags().StringVarP(&port, "port", "P", config.SERVER_PORT, "Server port")
}

func rdt_rcv(clientSocket *net.UDPConn) []byte {
	buf := make([]byte, config.CLIENT_RECV_LEN)
	_, err := clientSocket.Read(buf)
	checkErr(err)
	return buf
}
