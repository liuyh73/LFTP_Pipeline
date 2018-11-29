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
	"bytes"
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/liuyh73/LFTP_Pipeline/LFTP/config"
	"github.com/liuyh73/LFTP_Pipeline/LFTP/models"
	"github.com/spf13/cobra"
)

var lgetFile string

// lgetCmd represents the lget command
var lgetCmd = &cobra.Command{
	Use:   "lget",
	Short: "lget command helps us to get a file from server.",
	Long:  `We can use LFTP lget <file> to get a file from server.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("lget called")
		expectedseqnum := 1
		bufSize := 50
		var bufs bytes.Buffer
		var timer *time.Timer
		var sndpkt *models.Packet
		var wg sync.WaitGroup
		stopTimer := make(chan int)
		// 获取raddr
		serverAddr := host + ":" + port
		raddr, err := net.ResolveUDPAddr("udp", serverAddr)
		checkErr(err)
		// 获取客户端套接字
		// net.DialUDP("udp", localAddr *UDPAddr, remoteAddr *UDPAddr)
		clientSocket, err := net.DialUDP("udp", nil, raddr)
		checkErr(err)
		defer clientSocket.Close()
		lgetPacket := models.NewPacket(rune(0), rune(0), rune(bufSize), byte(1), byte(0), []byte("lget: "+lgetFile))
		fmt.Println(lgetFile)
		// 设置等待响应时间
		clientSocket.SetDeadline(time.Now().Add(10 * time.Second))
		// 向服务器发送请求
		_, err = clientSocket.Write(lgetPacket.ToBytes())
		checkErr(err)
		// 创建文件句柄
		outputFile, err := os.OpenFile(lgetFile, os.O_CREATE|os.O_TRUNC, 0600)
		checkErr(err)
		for {
			rcvpkt := &models.Packet{}
			buf := rdt_rcv(clientSocket)
			rcvpkt.FromBytes(buf)
			if rcvpkt.Seqnum == rune(expectedseqnum) && bufSize > 0 {
				bufSize -= 1
				sndpkt = models.NewPacket(rune(expectedseqnum), rune(expectedseqnum), rune(bufSize), byte(1), rcvpkt.Finished, buf)

				bufs.Write(sndpkt.ToBytes())
				expectedseqnum += 1
			}
			clientSocket.Write(sndpkt.ToBytes())
			if rcvpkt.Finished == byte(1) {
				timer = time.NewTimer(5 * time.Second)
				break
			}
		}
		wg.Add(1)
		// 如果超时，重新发送数据包sndpkt, 设置定时器
		go func() {
			defer wg.Done()
			for {
				select {
				case <-timer.C:
					fmt.Println("发送数据包0超时")
					clientSocket.Write(sndpkt.ToBytes())
					timer.Reset(5)
				case <-stopTimer:
					return
				}
			}
		}()
		// 等待ACK Finished
		for {
			rcvpkt := &models.Packet{}
			rcvpkt.FromBytes(rdt_rcv(clientSocket))
			if rcvpkt.Seqnum == rune(expectedseqnum) && rcvpkt.Finished == 1 {
				fmt.Println("接收ACK Finished")
				break
			}
		}
		stopTimer <- 1
		// ACK == 0
		// 取消定时器
		timer.Stop()
		wg.Wait()
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

func WriteDataToFile(clientSocket *net.UDPConn, outputFile *os.File, packetRcv *models.Packet) bool {
	// 收到下层的0或1，读取收到的数据包
	length, err := outputFile.Write(packetRcv.Data)
	fmt.Println("Read lenth: " + strconv.Itoa(length))
	checkErr(err)

	// 传输完成，判断是否传输完成
	if packetRcv.Finished == byte(1) {
		fmt.Println("end")
		packetSnd := models.NewPacket(byte(0), byte(packetRcv.PkgNum), byte(1), byte(1), []byte{})
		clientSocket.Write(packetSnd.ToBytes())
		return true
	}
	return false
}

func rdt_rcv(clientSocket *net.UDPConn) []byte {
	buf := make([]byte, config.CLIENT_RECV_LEN)
	_, err := clientSocket.Read(buf)
	checkErr(err)
	return buf
}
