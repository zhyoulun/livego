package rtmp

import (
	"encoding/json"
	"time"

	cmap "github.com/orcaman/concurrent-map"
	log "github.com/sirupsen/logrus"
	"github.com/zhyoulun/livego/av"
)

type StreamManager struct {
	streams cmap.ConcurrentMap //key
}

func NewStreamManager() *StreamManager {
	sm := &StreamManager{
		streams: cmap.New(),
	}
	go sm.CheckAlive()
	return sm
}

//处理推流
func (sm *StreamManager) HandleReader(r av.ReadCloser) {
	info := r.Info()
	log.Debugf("HandleReader: info[%v]", info)

	var stream *Stream
	s, ok := sm.streams.Get(info.Key)
	if stream, ok = s.(*Stream); ok {
		stream.TransStop() //如果已经有在推流，则停掉在推流

		ns := NewStream(info)
		stream.Copy(ns)
		stream = ns
		sm.streams.Set(info.Key, ns)
		log.Debugf("StreamManager.streams add reader: %+v", info.Key)
	} else {
		stream = NewStream(info)
		sm.streams.Set(info.Key, stream)
		log.Debugf("StreamManager.streams add reader: %+v", info.Key)
	}

	stream.AddReader(r)
}

//处理拉流
func (sm *StreamManager) HandleWriter(w av.WriteCloser) {
	info := w.Info()
	log.Debugf("HandleWriter: info[%v]", info)

	var s *Stream
	ok := sm.streams.Has(info.Key)
	if !ok {
		s = NewStream(info)
		sm.streams.Set(info.Key, s)
		log.Debugf("rtmpStream.streams add writer: %+v", info.Key)
		s.info = info
	} else {
		item, ok := sm.streams.Get(info.Key)
		if ok {
			s = item.(*Stream)
			s.AddWriter(w)
		}
	}
}

func (sm *StreamManager) GetStreams() cmap.ConcurrentMap {
	return sm.streams
}

func (sm *StreamManager) CheckAlive() {
	for {
		<-time.After(5 * time.Second)
		for item := range sm.streams.IterBuffered() {
			v := item.Val.(*Stream)
			if v.CheckAlive() == 0 {
				sm.streams.Remove(item.Key)
			}
		}
	}
}

func (sm *StreamManager) MarshalJSON() ([]byte, error) {
	type stream struct {
		Key             string `json:"key"`
		Url             string `json:"url"`
		SessionID       string `json:"session_id"`
		StreamId        uint32 `json:"stream_id"`
		VideoTotalBytes uint64 `json:"video_total_bytes"`
		VideoSpeed      uint64 `json:"video_speed"`
		AudioTotalBytes uint64 `json:"audio_total_bytes"`
		AudioSpeed      uint64 `json:"audio_speed"`
	}

	type streams struct {
		Publishers []stream `json:"publishers"`
		Players    []stream `json:"players"`
	}

	msgs := new(streams)
	for item := range sm.GetStreams().IterBuffered() {
		if s, ok := item.Val.(*Stream); ok {
			if s.GetReader() != nil {
				switch s.GetReader().(type) {
				case *VirReader:
					v := s.GetReader().(*VirReader)
					msg := stream{item.Key, v.Info().URL, v.Info().SessionID, v.ReadBWInfo.StreamId, v.ReadBWInfo.VideoDatainBytes, v.ReadBWInfo.VideoSpeedInBytesperMS,
						v.ReadBWInfo.AudioDatainBytes, v.ReadBWInfo.AudioSpeedInBytesperMS}
					msgs.Publishers = append(msgs.Publishers, msg)
				}
			}
		}
	}

	for item := range sm.GetStreams().IterBuffered() {
		ws := item.Val.(*Stream).GetSinks()
		for s := range ws.IterBuffered() {
			if pw, ok := s.Val.(*PackWriterCloser); ok {
				if pw.GetWriter() != nil {
					switch pw.GetWriter().(type) {
					case *VirWriter:
						v := pw.GetWriter().(*VirWriter)
						msg := stream{item.Key, v.Info().URL, v.Info().SessionID, v.WriteBWInfo.StreamId, v.WriteBWInfo.VideoDatainBytes, v.WriteBWInfo.VideoSpeedInBytesperMS,
							v.WriteBWInfo.AudioDatainBytes, v.WriteBWInfo.AudioSpeedInBytesperMS}
						msgs.Players = append(msgs.Players, msg)
					}
				}
			}
		}
	}

	return json.Marshal(msgs)
}
