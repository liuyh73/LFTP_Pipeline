package models

import (
	"bytes"
	"fmt"
)

type Head struct {
	Seqnum   rune
	Ack      rune
	Rwnd     rune
	Status   byte
	Finished byte
	Length   rune
}

type Body struct {
	Data []byte
}

type Packet struct {
	Head
	Body
}

// 初始化一个Packet包
func NewPacket(seqnum, ack, rwnd rune, status, finished byte, length rune, data []byte) *Packet {
	packet := &Packet{}
	packet.Seqnum = seqnum
	packet.Ack = ack
	packet.Rwnd = rwnd
	packet.Status = status
	packet.Finished = finished
	packet.Length = length
	packet.Data = data
	return packet
}

// 将Packet转化为[]byte（封包）
func (packet *Packet) ToBytes() []byte {
	var bytesBuf bytes.Buffer
	bytesBuf.WriteRune(packet.Seqnum)
	bytesBuf.WriteRune(packet.Ack)
	bytesBuf.WriteRune(packet.Rwnd)
	bytesBuf.WriteByte(packet.Status)
	bytesBuf.WriteByte(packet.Finished)
	bytesBuf.WriteRune(packet.Length)
	bytesBuf.Write(packet.Data)
	return bytesBuf.Bytes()
}

// 将[]byte解析到packet包中（拆包）
func (packet *Packet) FromBytes(buf []byte) {
	// buf = bytes.TrimRight(buf, "\x00")
	bytesBuf := bytes.NewBuffer(buf)
	var err error
	packet.Seqnum, _, err = bytesBuf.ReadRune()
	checkErr(err)
	packet.Ack, _, err = bytesBuf.ReadRune()
	checkErr(err)
	packet.Rwnd, _, err = bytesBuf.ReadRune()
	checkErr(err)
	packet.Status, err = bytesBuf.ReadByte()
	checkErr(err)
	packet.Finished, err = bytesBuf.ReadByte()
	checkErr(err)
	length, _, err := bytesBuf.ReadRune()
	checkErr(err)
	packet.Length = length
	packet.Data = bytesBuf.Next(int(length))
}

func checkErr(err error) {
	if err != nil {
		fmt.Println(err)
	}
}
