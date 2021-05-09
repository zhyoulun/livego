package flv

import (
	"fmt"
	"github.com/zhyoulun/livego/av"
)

var (
	ErrAvcEndSEQ = fmt.Errorf("avc end sequence")
)

type Demuxer struct {
}

func NewDemuxer() *Demuxer {
	return &Demuxer{}
}

func (d *Demuxer) DemuxH(p *av.Packet) error {
	var tag Tag
	_, err := tag.ParseMediaTagHeader(p.Data, p.IsVideo)
	if err != nil {
		return err
	}
	p.Header = &tag

	return nil
}
