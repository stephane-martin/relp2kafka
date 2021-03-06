package model

import (
	"net"
	"net/http"
	"sync"

	"github.com/stephane-martin/skewer/conf"
	"github.com/stephane-martin/skewer/utils"
)

type Stasher interface {
	Stash(m *FullMessage) (error, error)
}

type Reporter interface {
	Stasher
	Report(infos []ListenerInfo) error
}

type ListenerInfo struct {
	Port           int    `json:"port" msg:"port"`
	BindAddr       string `json:"bind_addr" mdg:"bind_addr"`
	UnixSocketPath string `json:"unix_socket_path" msg:"unix_socket_path"`
	Protocol       string `json:"protocol" msg:"protocol"`
}

type RawFileMessage struct {
	Decoder   conf.DecoderBaseConfig
	Hostname  string
	Directory string
	Glob      string
	Filename  string
	Line      []byte
	ConfID    utils.MyULID
}

type RawMessage struct {
	Decoder        conf.DecoderBaseConfig
	Client         string
	LocalPort      int
	UnixSocketPath string
	ConfID         utils.MyULID
}

type RawKafkaMessage struct {
	RawMessage
	ConsumerID uint32
	Message    []byte
	UID        utils.MyULID
	Topic      string
	Partition  int32
	Offset     int64
}

type RawTCPMessage struct {
	RawMessage
	Message []byte
	Txnr    int32
	ConnID  utils.MyULID
}

type RawUDPMessage struct {
	RawMessage
	Message [65536]byte
	Size    int
}

type DeferedRequest struct {
	UID     utils.MyULID
	Request *http.Request
}

var rawTCPPool = &sync.Pool{
	New: func() interface{} {
		return &RawTCPMessage{
			Message: make([]byte, 0, 4096),
		}
	},
}

var rawUDPPool = &sync.Pool{
	New: func() interface{} {
		return new(RawUDPMessage)
	},
}

func RawTCPFactory(message []byte) (raw *RawTCPMessage) {
	raw = rawTCPPool.Get().(*RawTCPMessage)
	if cap(raw.Message) < len(message) {
		raw.Message = make([]byte, 0, len(message))
	}
	raw.Message = raw.Message[:len(message)]
	copy(raw.Message, message)
	return raw
}

func RawTCPFree(raw *RawTCPMessage) {
	rawTCPPool.Put(raw)
}

func RawUDPFactory() (raw *RawUDPMessage) {
	return rawUDPPool.Get().(*RawUDPMessage)
}

func RawUDPFree(raw *RawUDPMessage) {
	rawUDPPool.Put(raw)
}

func (raw *RawUDPMessage) GetMessage() []byte {
	return raw.Message[:raw.Size]
}

func RawUDPFromConn(conn net.PacketConn) (raw *RawUDPMessage, remote net.Addr, err error) {
	raw = RawUDPFactory()
	raw.Size, remote, err = conn.ReadFrom(raw.Message[:])
	return raw, remote, err
}
