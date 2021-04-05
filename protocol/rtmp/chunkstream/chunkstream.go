package chunkstream

import (
	"encoding/binary"
	"fmt"
	"github.com/zhyoulun/livego/av"
	"github.com/zhyoulun/livego/utils/mio"
	"github.com/zhyoulun/livego/utils/pool"
)

type ChunkStream struct {
	Format    uint32
	TmpFormat uint32
	CSID      uint32
	Timestamp uint32
	Length    uint32
	TypeID    uint32
	StreamID  uint32
	timeDelta uint32
	exted     bool
	index     uint32
	remain    uint32
	got       bool
	Data      []byte
}

func (cs *ChunkStream) new(pool *pool.Pool) {
	cs.got = false
	cs.index = 0
	cs.remain = cs.Length
	cs.Data = pool.Get(int(cs.Length))
}

func (cs *ChunkStream) writeHeader(w *mio.ReadWriter) error {
	//Chunk Basic Header
	h := cs.Format << 6
	switch {
	case cs.CSID < 64:
		h |= cs.CSID
		w.WriteUintBE(h, 1)
	case cs.CSID-64 < 256:
		h |= 0
		w.WriteUintBE(h, 1)
		w.WriteUintLE(cs.CSID-64, 1)
	case cs.CSID-64 < 65536:
		h |= 1
		w.WriteUintBE(h, 1)
		w.WriteUintLE(cs.CSID-64, 2)
	}
	//Chunk Message Header
	ts := cs.Timestamp
	if cs.Format == 3 {
		goto END
	}
	if cs.Timestamp > 0xffffff {
		ts = 0xffffff
	}
	w.WriteUintBE(ts, 3)
	if cs.Format == 2 {
		goto END
	}
	if cs.Length > 0xffffff {
		return fmt.Errorf("length=%d", cs.Length)
	}
	w.WriteUintBE(cs.Length, 3)
	w.WriteUintBE(cs.TypeID, 1)
	if cs.Format == 1 {
		goto END
	}
	w.WriteUintLE(cs.StreamID, 4)
END:
	//Extended Timestamp
	if ts >= 0xffffff {
		w.WriteUintBE(cs.Timestamp, 4)
	}
	return w.WriteError()
}

func (cs *ChunkStream) Full() bool {
	return cs.got
}

func (cs *ChunkStream) WriteChunk(w *mio.ReadWriter, chunkSize int) error {
	if cs.TypeID == av.TAG_AUDIO {
		cs.CSID = 4
	} else if cs.TypeID == av.TAG_VIDEO ||
		cs.TypeID == av.TAG_SCRIPTDATAAMF0 ||
		cs.TypeID == av.TAG_SCRIPTDATAAMF3 {
		cs.CSID = 6
	}

	totalLen := uint32(0)
	numChunks := cs.Length / uint32(chunkSize)
	for i := uint32(0); i <= numChunks; i++ {
		if totalLen == cs.Length {
			break
		}
		if i == 0 {
			cs.Format = uint32(0)
		} else {
			cs.Format = uint32(3)
		}
		if err := cs.writeHeader(w); err != nil {
			return err
		}
		inc := uint32(chunkSize)
		start := uint32(i) * uint32(chunkSize)
		if uint32(len(cs.Data))-start <= inc {
			inc = uint32(len(cs.Data)) - start
		}
		totalLen += inc
		end := start + inc
		buf := cs.Data[start:end]
		if _, err := w.Write(buf); err != nil {
			return err
		}
	}

	return nil

}

func (cs *ChunkStream) ReadChunk(r *mio.ReadWriter, chunkSize uint32, pool *pool.Pool) error {
	if cs.remain != 0 && cs.TmpFormat != 3 {
		return fmt.Errorf("inlaid remain = %d", cs.remain)
	}
	switch cs.CSID {
	case 0:
		id, _ := r.ReadUintLE(1)
		cs.CSID = id + 64
	case 1:
		id, _ := r.ReadUintLE(2)
		cs.CSID = id + 64
	}

	switch cs.TmpFormat {
	case 0:
		cs.Format = cs.TmpFormat
		cs.Timestamp, _ = r.ReadUintBE(3)
		cs.Length, _ = r.ReadUintBE(3)
		cs.TypeID, _ = r.ReadUintBE(1)
		cs.StreamID, _ = r.ReadUintLE(4)
		if cs.Timestamp == 0xffffff {
			cs.Timestamp, _ = r.ReadUintBE(4)
			cs.exted = true
		} else {
			cs.exted = false
		}
		cs.new(pool)
	case 1:
		cs.Format = cs.TmpFormat
		timeStamp, _ := r.ReadUintBE(3)
		cs.Length, _ = r.ReadUintBE(3)
		cs.TypeID, _ = r.ReadUintBE(1)
		if timeStamp == 0xffffff {
			timeStamp, _ = r.ReadUintBE(4)
			cs.exted = true
		} else {
			cs.exted = false
		}
		cs.timeDelta = timeStamp
		cs.Timestamp += timeStamp
		cs.new(pool)
	case 2:
		cs.Format = cs.TmpFormat
		timeStamp, _ := r.ReadUintBE(3)
		if timeStamp == 0xffffff {
			timeStamp, _ = r.ReadUintBE(4)
			cs.exted = true
		} else {
			cs.exted = false
		}
		cs.timeDelta = timeStamp
		cs.Timestamp += timeStamp
		cs.new(pool)
	case 3:
		if cs.remain == 0 {
			switch cs.Format {
			case 0:
				if cs.exted {
					timestamp, _ := r.ReadUintBE(4)
					cs.Timestamp = timestamp
				}
			case 1, 2:
				var timedet uint32
				if cs.exted {
					timedet, _ = r.ReadUintBE(4)
				} else {
					timedet = cs.timeDelta
				}
				cs.Timestamp += timedet
			}
			cs.new(pool)
		} else {
			if cs.exted {
				b, err := r.Peek(4)
				if err != nil {
					return err
				}
				tmpts := binary.BigEndian.Uint32(b)
				if tmpts == cs.Timestamp {
					r.Discard(4)
				}
			}
		}
	default:
		return fmt.Errorf("invalid format=%d", cs.Format)
	}
	size := int(cs.remain)
	if size > int(chunkSize) {
		size = int(chunkSize)
	}

	buf := cs.Data[cs.index : cs.index+uint32(size)]
	if _, err := r.Read(buf); err != nil {
		return err
	}
	cs.index += uint32(size)
	cs.remain -= uint32(size)
	if cs.remain == 0 {
		cs.got = true
	}

	return r.ReadError()
}
