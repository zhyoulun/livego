package rtmp

import (
	"fmt"
	cmap "github.com/orcaman/concurrent-map"
	log "github.com/sirupsen/logrus"
	"github.com/zhyoulun/livego/av"
	"github.com/zhyoulun/livego/protocol/rtmp/cache"
)

type PackWriterCloser struct {
	init bool
	w    av.WriteCloser
}

func (p *PackWriterCloser) GetWriter() av.WriteCloser {
	return p.w
}

type Stream struct {
	isStart bool
	cache   *cache.Cache
	r       av.ReadCloser
	sinks   cmap.ConcurrentMap
	info    av.Info
}

func NewStream(info av.Info) *Stream {
	return &Stream{
		info:  info,
		cache: cache.NewCache(),
		sinks: cmap.New(),
	}
}

func (s *Stream) GetReader() av.ReadCloser {
	return s.r
}

func (s *Stream) GetSinks() cmap.ConcurrentMap {
	return s.sinks
}

func (s *Stream) Copy(dst *Stream) {
	for item := range s.sinks.IterBuffered() {
		v := item.Val.(*PackWriterCloser)
		s.sinks.Remove(item.Key)
		v.w.CalcBaseTimestamp()
		dst.AddWriter(v.w)
	}
}

func (s *Stream) AddReader(r av.ReadCloser) {
	s.r = r
	go s.TransStart()
}

func (s *Stream) AddWriter(w av.WriteCloser) {
	info := w.Info()
	pw := &PackWriterCloser{w: w}
	s.sinks.Set(info.SessionID, pw)
}

func (s *Stream) TransStart() {
	s.isStart = true
	var p av.Packet

	log.Debugf("TransStart: %v", s.info)

	for {
		if !s.isStart {
			s.closeInter()
			return
		}
		err := s.r.Read(&p)
		if err != nil {
			s.closeInter()
			s.isStart = false
			return
		}

		//publish先写到cache中
		s.cache.Write(p)

		for item := range s.sinks.IterBuffered() {
			v := item.Val.(*PackWriterCloser)
			//如果还没有给sink发过包，则将cache中的数据发送过去
			if !v.init {
				//log.Debugf("cache.send: %v", v.w.Info())
				if err = s.cache.Send(v.w); err != nil {
					log.Debugf("[%s] send cache packet error: %v, remove", v.w.Info(), err)
					s.sinks.Remove(item.Key)
					continue
				}
				v.init = true
			} else {
				new_packet := p
				//writeType := reflect.TypeOf(v.w)
				//log.Debugf("w.Write: type=%v, %v", writeType, v.w.Info())
				if err = v.w.Write(&new_packet); err != nil {
					log.Debugf("[%s] write packet error: %v, remove", v.w.Info(), err)
					s.sinks.Remove(item.Key)
				}
			}
		}
	}
}

func (s *Stream) TransStop() {
	log.Debugf("TransStop: %s", s.info.Key)

	if s.isStart && s.r != nil {
		s.r.Close(fmt.Errorf("stop old"))
	}

	s.isStart = false
}

func (s *Stream) CheckAlive() (n int) {
	if s.r != nil && s.isStart {
		if s.r.Alive() {
			n++
		} else {
			s.r.Close(fmt.Errorf("read timeout"))
		}
	}
	for item := range s.sinks.IterBuffered() {
		v := item.Val.(*PackWriterCloser)
		if v.w != nil {
			if !v.w.Alive() && s.isStart {
				s.sinks.Remove(item.Key)
				v.w.Close(fmt.Errorf("write timeout"))
				continue
			}
			n++
		}

	}
	return
}

func (s *Stream) closeInter() {
	if s.r != nil {
		//s.StopStaticPush()
		log.Debugf("[%v] publisher closed", s.r.Info())
	}

	for item := range s.sinks.IterBuffered() {
		v := item.Val.(*PackWriterCloser)
		if v.w != nil {
			if v.w.Info().IsInterval() {
				v.w.Close(fmt.Errorf("closed"))
				s.sinks.Remove(item.Key)
				log.Debugf("[%v] player closed and remove\n", v.w.Info())
			}
		}
	}
}
