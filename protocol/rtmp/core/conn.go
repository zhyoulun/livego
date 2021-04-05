package core

import (
	"encoding/binary"
	"github.com/zhyoulun/livego/protocol/rtmp/c"
	"github.com/zhyoulun/livego/protocol/rtmp/chunkstream"
	"github.com/zhyoulun/livego/protocol/rtmp/message"
	"github.com/zhyoulun/livego/utils/mio"
	"net"
	"time"

	"github.com/zhyoulun/livego/utils/pool"
)

type Conn struct {
	Conn                net.Conn
	chunkSize           uint32
	remoteChunkSize     uint32
	windowAckSize       uint32
	remoteWindowAckSize uint32
	received            uint32
	ackReceived         uint32
	rw                  *mio.ReadWriter
	pool                *pool.Pool
	chunkStreams        map[uint32]chunkstream.ChunkStream
}

func NewConn(c net.Conn, bufferSize int) *Conn {
	return &Conn{
		Conn:                c,
		chunkSize:           128,
		remoteChunkSize:     128,
		windowAckSize:       2500000,
		remoteWindowAckSize: 2500000,
		pool:                pool.NewPool(),
		rw:                  mio.NewReadWriter(c, bufferSize),
		chunkStreams:        make(map[uint32]chunkstream.ChunkStream),
	}
}

func (conn *Conn) Read(c *chunkstream.ChunkStream) error {
	for {
		h, _ := conn.rw.ReadUintBE(1)
		format := h >> 6
		csID := h & 0x3f
		cs, ok := conn.chunkStreams[csID]
		if !ok {
			cs = chunkstream.ChunkStream{}
			conn.chunkStreams[csID] = cs
		}
		cs.TmpFormat = format
		cs.CSID = csID
		err := cs.ReadChunk(conn.rw, conn.remoteChunkSize, conn.pool)
		if err != nil {
			return err
		}
		conn.chunkStreams[csID] = cs
		if cs.Full() {
			*c = cs
			break
		}
	}

	conn.handleProtocolControlMessage(c)

	conn.ack(c.Length)

	return nil
}

func (conn *Conn) Write(cs *chunkstream.ChunkStream) error {
	if cs.TypeID == c.IDSetChunkSize {
		conn.chunkSize = binary.BigEndian.Uint32(cs.Data)
	}
	return cs.WriteChunk(conn.rw, int(conn.chunkSize))
}

func (conn *Conn) Flush() error {
	return conn.rw.Flush()
}

func (conn *Conn) Close() error {
	return conn.Conn.Close()
}

func (conn *Conn) RemoteAddr() net.Addr {
	return conn.Conn.RemoteAddr()
}

func (conn *Conn) LocalAddr() net.Addr {
	return conn.Conn.LocalAddr()
}

func (conn *Conn) SetDeadline(t time.Time) error {
	return conn.Conn.SetDeadline(t)
}

func (conn *Conn) handleProtocolControlMessage(cs *chunkstream.ChunkStream) {
	if cs.TypeID == c.IDSetChunkSize {
		conn.remoteChunkSize = binary.BigEndian.Uint32(cs.Data)
	} else if cs.TypeID == c.IDWindowAckSize {
		conn.remoteWindowAckSize = binary.BigEndian.Uint32(cs.Data)
	}
}

func (conn *Conn) ack(size uint32) {
	conn.received += size
	conn.ackReceived += size
	if conn.received >= 0xf0000000 {
		conn.received = 0
	}
	if conn.ackReceived >= conn.remoteWindowAckSize {
		cs := message.NewAck(conn.ackReceived)
		cs.WriteChunk(conn.rw, int(conn.chunkSize))
		conn.ackReceived = 0
	}
}

const (
	streamBegin      uint32 = 0
	streamEOF        uint32 = 1
	streamDry        uint32 = 2
	setBufferLen     uint32 = 3
	streamIsRecorded uint32 = 4
	pingRequest      uint32 = 6
	pingResponse     uint32 = 7
)

func (conn *Conn) SetBegin() {
	ret := message.NewUserControlMessage(streamBegin, 4)
	for i := 0; i < 4; i++ {
		ret.Data[2+i] = byte(1 >> uint32((3-i)*8) & 0xff)
	}
	conn.Write(&ret)
}

func (conn *Conn) SetRecorded() {
	ret := message.NewUserControlMessage(streamIsRecorded, 4)
	for i := 0; i < 4; i++ {
		ret.Data[2+i] = byte(1 >> uint32((3-i)*8) & 0xff)
	}
	conn.Write(&ret)
}
