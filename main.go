package main

import (
	"fmt"
	"github.com/zhyoulun/livego/configure"
	"github.com/zhyoulun/livego/protocol/api"
	"github.com/zhyoulun/livego/protocol/rtmp"
	"net"
	"path"
	"runtime"
	"time"

	log "github.com/sirupsen/logrus"
)

var VERSION = "master"

//func startHls() *hls.Server {
//	hlsAddr := configure.Config.GetString("hls_addr")
//	hlsListen, err := net.Listen("tcp", hlsAddr)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	hlsServer := hls.NewServer()
//	go func() {
//		defer func() {
//			if r := recover(); r != nil {
//				log.Error("HLS server panic: ", r)
//			}
//		}()
//		log.Info("HLS listen On ", hlsAddr)
//		hlsServer.Serve(hlsListen)
//	}()
//	return hlsServer
//}

var rtmpAddr string

func startRtmp(stream *rtmp.RtmpStream) {
	rtmpAddr = configure.Config.GetString("rtmp_addr")

	rtmpListen, err := net.Listen("tcp", rtmpAddr)
	if err != nil {
		log.Fatal(err)
	}

	var rtmpServer *rtmp.Server

	rtmpServer = rtmp.NewRtmpServer(stream, nil)

	defer func() {
		if r := recover(); r != nil {
			log.Error("RTMP server panic: ", r)
		}
	}()
	log.Info("RTMP Listen On ", rtmpAddr)
	rtmpServer.Serve(rtmpListen)
}

//func startHTTPFlv(stream *rtmp.RtmpStream) {
//	httpflvAddr := configure.Config.GetString("httpflv_addr")
//
//	flvListen, err := net.Listen("tcp", httpflvAddr)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	hdlServer := httpflv.NewServer(stream)
//	go func() {
//		defer func() {
//			if r := recover(); r != nil {
//				log.Error("HTTP-FLV server panic: ", r)
//			}
//		}()
//		log.Info("HTTP-FLV listen On ", httpflvAddr)
//		hdlServer.Serve(flvListen)
//	}()
//}

func startAPI(stream *rtmp.RtmpStream) {
	apiAddr := configure.Config.GetString("api_addr")

	if apiAddr != "" {
		opListen, err := net.Listen("tcp", apiAddr)
		if err != nil {
			log.Fatal(err)
		}
		opServer := api.NewServer(stream, rtmpAddr)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Error("HTTP-API server panic: ", r)
				}
			}()
			log.Info("HTTP-API listen On ", apiAddr)
			opServer.Serve(opListen)
		}()
	}
}

func init() {
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
		CallerPrettyfier: func(f *runtime.Frame) (string, string) {
			filename := path.Base(f.File)
			return fmt.Sprintf("%s()", f.Function), fmt.Sprintf(" %s:%d", filename, f.Line)
		},
	})
	log.SetLevel(log.DebugLevel)//打开debug日志开关
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			log.Error("livego panic: ", r)
			time.Sleep(1 * time.Second)
		}
	}()

	log.Infof(`
     _     _            ____       
    | |   (_)_   _____ / ___| ___  
    | |   | \ \ / / _ \ |  _ / _ \ 
    | |___| |\ V /  __/ |_| | (_) |
    |_____|_| \_/ \___|\____|\___/ 
        version: %s
	`, VERSION)

	stream := rtmp.NewRtmpStream()
	//hlsServer := startHls()
	//startHTTPFlv(stream)
	startAPI(stream)

	startRtmp(stream)
}
