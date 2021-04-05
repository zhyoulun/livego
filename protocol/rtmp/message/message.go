package message

import (
	"github.com/zhyoulun/livego/protocol/rtmp/c"
	"github.com/zhyoulun/livego/protocol/rtmp/chunkstream"
	"github.com/zhyoulun/livego/utils/pio"
)

//protocol control message
func NewAck(size uint32) chunkstream.ChunkStream {
	return newProtocolControlMessage(c.MessageTypeIDAck, 4, size)
}

//protocol control message
func NewSetChunkSize(size uint32) chunkstream.ChunkStream {
	return newProtocolControlMessage(c.MessageTypeIDSetChunkSize, 4, size)
}

//protocol control message
func NewWindowAckSize(size uint32) chunkstream.ChunkStream {
	return newProtocolControlMessage(c.MessageTypeIDWindowAckSize, 4, size)
}

//protocol control message
func NewSetPeerBandwidth(size uint32) chunkstream.ChunkStream {
	ret := newProtocolControlMessage(c.MessageTypeIDSetPeerBandwidth, 5, size)
	ret.Data[4] = 2
	return ret
}

func newProtocolControlMessage(id, size, value uint32) chunkstream.ChunkStream {
	ret := chunkstream.ChunkStream{
		Format:   0,
		CSID:     2,
		TypeID:   id,
		StreamID: 0,
		Length:   size,
		Data:     make([]byte, size),
	}
	pio.PutU32BE(ret.Data[:size], value)
	return ret
}

//user control message
//+------------------------------+-------------------------
//|     Event Type ( 2- bytes )  | Event Data
//+------------------------------+-------------------------
//Pay load for the ‘User Control Message’.
func NewUserControlMessage(eventType, buflen uint32) chunkstream.ChunkStream {
	var ret chunkstream.ChunkStream
	buflen += 2
	ret = chunkstream.ChunkStream{
		Format:   0,
		CSID:     2,
		TypeID:   4,
		StreamID: 1,
		Length:   buflen,
		Data:     make([]byte, buflen),
	}
	ret.Data[0] = byte(eventType >> 8 & 0xff)
	ret.Data[1] = byte(eventType & 0xff)
	return ret
}
