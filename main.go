package main

import (
	"fmt"
	"github.com/zhyoulun/livego/protocol/api"
	"github.com/zhyoulun/livego/protocol/rtmp"
	"net"
	"path"
	"runtime"
	"time"

	log "github.com/sirupsen/logrus"
)

var VERSION = "master"

var rtmpAddr string

func startRtmp(stream *rtmp.StreamManager) {
	rtmpAddr = "127.0.0.1:1935"

	ln, err := net.Listen("tcp", rtmpAddr)
	if err != nil {
		log.Fatal(err)
	}

	var rtmpServer *rtmp.Server

	rtmpServer = rtmp.NewRtmpServer(stream)

	defer func() {
		if r := recover(); r != nil {
			log.Error("RTMP server panic: ", r)
		}
	}()
	log.Info("RTMP Listen On ", rtmpAddr)
	log.Errorf("rtmpServer Serve err: %+v", rtmpServer.Serve(ln))
}

func startAPI(stream *rtmp.StreamManager) {
	apiAddr := "127.0.0.1:8080"

	ln, err := net.Listen("tcp", apiAddr)
	if err != nil {
		log.Fatal(err)
	}
	apiServer := api.NewServer(stream, rtmpAddr)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error("HTTP-API server panic: ", r)
			}
		}()
		log.Info("HTTP-API listen On ", apiAddr)
		log.Errorf("apiServer Serve err: %+v", apiServer.Serve(ln))
	}()
}

func Init() error {
	{
		log.SetFormatter(&log.TextFormatter{
			FullTimestamp: true,
			CallerPrettyfier: func(f *runtime.Frame) (string, string) {
				filename := path.Base(f.File)
				return fmt.Sprintf("%s()", f.Function), fmt.Sprintf(" %s:%d", filename, f.Line)
			},
		})
		log.SetLevel(log.DebugLevel) //打开debug日志开关
		log.SetReportCaller(true)    //在输出日志中添加文件名和方法信息
	}

	return nil
}

func main() {
	if err := Init(); err != nil {
		panic(err)
	}

	defer func() {
		if r := recover(); r != nil {
			log.Error("livego panic: ", r)
			time.Sleep(1 * time.Second)
		}
	}()

	stream := rtmp.NewStreamManager()
	startAPI(stream)

	startRtmp(stream)
}
