package rtmp

import (
	"fmt"
	"github.com/zhyoulun/livego/protocol/rtmp/chunkstream"
	"github.com/zhyoulun/livego/utils"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/zhyoulun/livego/av"
	"github.com/zhyoulun/livego/container/flv"
	"github.com/zhyoulun/livego/protocol/rtmp/core"

	log "github.com/sirupsen/logrus"
)

const (
	maxQueueNum           = 1024
	SAVE_STATICS_INTERVAL = 5000
)

var (
	//readTimeout  = configure.Config.GetInt("read_timeout")
	//writeTimeout = configure.Config.GetInt("write_timeout")
	writeTimeout = 1 //读写数据的超时时间，这里设置为1s
)

type Client struct {
	handler av.Handler
	getter  av.GetWriter
}

type Server struct {
	handler av.Handler
}

func NewRtmpServer(h av.Handler) *Server {
	return &Server{
		handler: h,
	}
}

func (s *Server) Serve(ln net.Listener) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Error("rtmp serve panic: ", r)
		}
	}()

	for {
		var netConn net.Conn
		netConn, err = ln.Accept()
		if err != nil {
			return
		}
		rtmpConn := core.NewConn(netConn, 4*1024)
		log.Debug("new client, connect remote: ", rtmpConn.RemoteAddr().String(),
			"local:", rtmpConn.LocalAddr().String())
		go s.handleConn(rtmpConn)
	}
}

func (s *Server) handleConn(conn *core.Conn) {
	//握手
	if err := conn.HandshakeServer(); err != nil {
		log.Error("handleConn HandshakeServer err: ", err)
		if err = conn.Close(); err != nil {
			log.Error("conn Close err: ", err)
		}
		return
	}
	connServer := core.NewConnServer(conn)

	if err := connServer.ReadMsg(); err != nil {
		log.Error("handleConn read msg err: ", err)
		if err = conn.Close(); err != nil {
			log.Error("conn Close err: ", err)
		}
		return
	}

	log.Debugf("handleConn: IsPublisher=%v", connServer.IsPublisher())
	if connServer.IsPublisher() {
		reader := NewVirReader(connServer)
		s.handler.HandleReader(reader)
		log.Debugf("new publisher: %+v", reader.Info())
	} else {
		writer := NewVirWriter(connServer)
		log.Debugf("new player: %+v", writer.Info())
		s.handler.HandleWriter(writer)
	}
}

type GetInFo interface {
	GetURL() string
}

type StreamReadWriteCloser interface {
	GetInFo
	Close(error)
	Write(chunkstream.ChunkStream) error
	Read(c *chunkstream.ChunkStream) error
	Flush() error
}

type StaticsBW struct {
	StreamId               uint32
	VideoDatainBytes       uint64
	LastVideoDatainBytes   uint64
	VideoSpeedInBytesperMS uint64

	AudioDatainBytes       uint64
	LastAudioDatainBytes   uint64
	AudioSpeedInBytesperMS uint64

	LastTimestamp int64
}

type VirWriter struct {
	sessionID string
	closed    bool
	av.RWBaser
	conn        StreamReadWriteCloser
	packetQueue chan *av.Packet
	WriteBWInfo StaticsBW
}

func NewVirWriter(conn StreamReadWriteCloser) *VirWriter {
	ret := &VirWriter{
		sessionID:   utils.GenSessionIDString(),
		conn:        conn,
		RWBaser:     av.NewRWBaser(time.Second * time.Duration(writeTimeout)),
		packetQueue: make(chan *av.Packet, maxQueueNum),
		WriteBWInfo: StaticsBW{0, 0, 0, 0, 0, 0, 0, 0},
	}

	go ret.Check()
	go func() {
		err := ret.SendPacket()
		if err != nil {
			log.Warning(err)
		}
	}()
	return ret
}

func (v *VirWriter) SaveStatics(streamid uint32, length uint64, isVideoFlag bool) {
	nowInMS := int64(time.Now().UnixNano() / 1e6)

	v.WriteBWInfo.StreamId = streamid
	if isVideoFlag {
		v.WriteBWInfo.VideoDatainBytes = v.WriteBWInfo.VideoDatainBytes + length
	} else {
		v.WriteBWInfo.AudioDatainBytes = v.WriteBWInfo.AudioDatainBytes + length
	}

	if v.WriteBWInfo.LastTimestamp == 0 {
		v.WriteBWInfo.LastTimestamp = nowInMS
	} else if (nowInMS - v.WriteBWInfo.LastTimestamp) >= SAVE_STATICS_INTERVAL {
		diffTimestamp := (nowInMS - v.WriteBWInfo.LastTimestamp) / 1000

		v.WriteBWInfo.VideoSpeedInBytesperMS = (v.WriteBWInfo.VideoDatainBytes - v.WriteBWInfo.LastVideoDatainBytes) * 8 / uint64(diffTimestamp) / 1000
		v.WriteBWInfo.AudioSpeedInBytesperMS = (v.WriteBWInfo.AudioDatainBytes - v.WriteBWInfo.LastAudioDatainBytes) * 8 / uint64(diffTimestamp) / 1000

		v.WriteBWInfo.LastVideoDatainBytes = v.WriteBWInfo.VideoDatainBytes
		v.WriteBWInfo.LastAudioDatainBytes = v.WriteBWInfo.AudioDatainBytes
		v.WriteBWInfo.LastTimestamp = nowInMS
	}
}

func (v *VirWriter) Check() {
	var c chunkstream.ChunkStream
	for {
		if err := v.conn.Read(&c); err != nil {
			v.Close(err)
			return
		}
	}
}

func (v *VirWriter) DropPacket(pktQue chan *av.Packet, info av.Info) {
	log.Warningf("[%v] packet queue max!!!", info)
	for i := 0; i < maxQueueNum-84; i++ {
		tmpPkt, ok := <-pktQue
		// try to don't drop audio
		if ok && tmpPkt.IsAudio {
			if len(pktQue) > maxQueueNum-2 {
				log.Debug("drop audio pkt")
				<-pktQue
			} else {
				pktQue <- tmpPkt
			}

		}

		if ok && tmpPkt.IsVideo {
			videoPkt, ok := tmpPkt.Header.(av.VideoPacketHeader)
			// dont't drop sps config and dont't drop key frame
			if ok && (videoPkt.IsSeq() || videoPkt.IsKeyFrame()) {
				pktQue <- tmpPkt
			}
			if len(pktQue) > maxQueueNum-10 {
				log.Debug("drop video pkt")
				<-pktQue
			}
		}

	}
	log.Debug("packet queue len: ", len(pktQue))
}

//
func (v *VirWriter) Write(p *av.Packet) (err error) {
	err = nil

	if v.closed {
		err = fmt.Errorf("VirWriter closed")
		return
	}
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("VirWriter has already been closed:%v", e)
		}
	}()
	if len(v.packetQueue) >= maxQueueNum-24 {
		v.DropPacket(v.packetQueue, v.Info())
	} else {
		v.packetQueue <- p
	}

	return
}

func (v *VirWriter) SendPacket() error {
	var cs chunkstream.ChunkStream
	for {
		p, ok := <-v.packetQueue
		if ok {
			cs.Data = p.Data
			cs.Length = uint32(len(p.Data))
			cs.StreamID = p.StreamID
			cs.Timestamp = p.TimeStamp
			cs.Timestamp += v.BaseTimeStamp()

			if p.IsVideo {
				cs.TypeID = av.TAG_VIDEO
			} else {
				if p.IsMetadata {
					cs.TypeID = av.TAG_SCRIPTDATAAMF0
				} else {
					cs.TypeID = av.TAG_AUDIO
				}
			}

			v.SaveStatics(p.StreamID, uint64(cs.Length), p.IsVideo)
			v.SetPreTime("write")
			v.RecTimeStamp(cs.Timestamp, cs.TypeID)
			err := v.conn.Write(cs)
			if err != nil {
				v.closed = true
				return err
			}
			v.conn.Flush()
		} else {
			return fmt.Errorf("closed")
		}

	}
	return nil
}

func (v *VirWriter) Info() (ret av.Info) {
	URL := v.conn.GetURL()
	ret.URL = URL
	_url, err := url.Parse(URL)
	if err != nil {
		log.Warning(err)
	}
	ret.Key = strings.TrimLeft(_url.Path, "/")
	ret.Inter = true
	ret.SessionID = v.sessionID
	return
}

func (v *VirWriter) Close(err error) {
	log.Warning("player ", v.Info(), "closed: "+err.Error())
	if !v.closed {
		close(v.packetQueue)
	}
	v.closed = true
	v.conn.Close(err)
}

type VirReader struct {
	sessionID string
	av.RWBaser
	demuxer    *flv.Demuxer
	conn       StreamReadWriteCloser
	ReadBWInfo StaticsBW
}

func NewVirReader(conn StreamReadWriteCloser) *VirReader {
	log.Debugf("writeTimeout: %d", writeTimeout)
	return &VirReader{
		sessionID:  utils.GenSessionIDString(),
		conn:       conn,
		RWBaser:    av.NewRWBaser(time.Second * time.Duration(writeTimeout)),
		demuxer:    flv.NewDemuxer(),
		ReadBWInfo: StaticsBW{0, 0, 0, 0, 0, 0, 0, 0},
	}
}

func (v *VirReader) SaveStatics(streamid uint32, length uint64, isVideoFlag bool) {
	nowInMS := int64(time.Now().UnixNano() / 1e6)

	v.ReadBWInfo.StreamId = streamid
	if isVideoFlag {
		v.ReadBWInfo.VideoDatainBytes = v.ReadBWInfo.VideoDatainBytes + length
	} else {
		v.ReadBWInfo.AudioDatainBytes = v.ReadBWInfo.AudioDatainBytes + length
	}

	if v.ReadBWInfo.LastTimestamp == 0 {
		v.ReadBWInfo.LastTimestamp = nowInMS
	} else if (nowInMS - v.ReadBWInfo.LastTimestamp) >= SAVE_STATICS_INTERVAL {
		diffTimestamp := (nowInMS - v.ReadBWInfo.LastTimestamp) / 1000

		//log.Printf("now=%d, last=%d, diff=%d", nowInMS, v.ReadBWInfo.LastTimestamp, diffTimestamp)
		v.ReadBWInfo.VideoSpeedInBytesperMS = (v.ReadBWInfo.VideoDatainBytes - v.ReadBWInfo.LastVideoDatainBytes) * 8 / uint64(diffTimestamp) / 1000
		v.ReadBWInfo.AudioSpeedInBytesperMS = (v.ReadBWInfo.AudioDatainBytes - v.ReadBWInfo.LastAudioDatainBytes) * 8 / uint64(diffTimestamp) / 1000

		v.ReadBWInfo.LastVideoDatainBytes = v.ReadBWInfo.VideoDatainBytes
		v.ReadBWInfo.LastAudioDatainBytes = v.ReadBWInfo.AudioDatainBytes
		v.ReadBWInfo.LastTimestamp = nowInMS
	}
}

func (v *VirReader) Read(p *av.Packet) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Warning("rtmp read packet panic: ", r)
		}
	}()

	v.SetPreTime("read")

	//get chunk stream
	var cs chunkstream.ChunkStream
	for {
		err = v.conn.Read(&cs)
		if err != nil {
			return err
		}
		if cs.TypeID == av.TAG_AUDIO ||
			cs.TypeID == av.TAG_VIDEO ||
			cs.TypeID == av.TAG_SCRIPTDATAAMF0 ||
			cs.TypeID == av.TAG_SCRIPTDATAAMF3 {
			break
		}
	}

	//change chunk stream to av packet
	p.IsAudio = cs.TypeID == av.TAG_AUDIO
	p.IsVideo = cs.TypeID == av.TAG_VIDEO
	p.IsMetadata = cs.TypeID == av.TAG_SCRIPTDATAAMF0 || cs.TypeID == av.TAG_SCRIPTDATAAMF3
	p.StreamID = cs.StreamID
	p.Data = cs.Data
	p.TimeStamp = cs.Timestamp

	v.SaveStatics(p.StreamID, uint64(len(p.Data)), p.IsVideo)

	//
	v.demuxer.DemuxH(p)

	return err
}

func (v *VirReader) Info() (ret av.Info) {
	URL := v.conn.GetURL()
	ret.URL = URL
	_url, err := url.Parse(URL)
	if err != nil {
		log.Warning(err)
	}
	ret.Key = strings.TrimLeft(_url.Path, "/")
	ret.SessionID = v.sessionID
	return
}

func (v *VirReader) Close(err error) {
	log.Debug("publisher ", v.Info(), "closed: "+err.Error())
	v.conn.Close(err)
}
