package api

import (
	"encoding/json"
	"net"
	"net/http"

	jwtmiddleware "github.com/auth0/go-jwt-middleware"
	"github.com/dgrijalva/jwt-go"
	log "github.com/sirupsen/logrus"
	"github.com/zhyoulun/livego/av"
	"github.com/zhyoulun/livego/configure"
	"github.com/zhyoulun/livego/protocol/rtmp"
)

type Response struct {
	w      http.ResponseWriter
	Status int         `json:"status"`
	Data   interface{} `json:"data"`
}

func (r *Response) SendJson() (int, error) {
	resp, _ := json.Marshal(r)
	r.w.Header().Set("Content-Type", "application/json")
	r.w.WriteHeader(r.Status)
	return r.w.Write(resp)
}

type Operation struct {
	Method string `json:"method"`
	URL    string `json:"url"`
	Stop   bool   `json:"stop"`
}

type OperationChange struct {
	Method    string `json:"method"`
	SourceURL string `json:"source_url"`
	TargetURL string `json:"target_url"`
	Stop      bool   `json:"stop"`
}

type ClientInfo struct {
	url              string
	rtmpRemoteClient *rtmp.Client
	rtmpLocalClient  *rtmp.Client
}

type Server struct {
	handler av.Handler
	//session  map[string]*rtmprelay.RtmpRelay
	rtmpAddr string
}

func NewServer(h av.Handler, rtmpAddr string) *Server {
	return &Server{
		handler: h,
		//session:  make(map[string]*rtmprelay.RtmpRelay),
		rtmpAddr: rtmpAddr,
	}
}

func JWTMiddleware(next http.Handler) http.Handler {
	isJWT := len(configure.Config.GetString("jwt.secret")) > 0
	if !isJWT {
		return next
	}

	log.Info("Using JWT middleware")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var algorithm jwt.SigningMethod
		if len(configure.Config.GetString("jwt.algorithm")) > 0 {
			algorithm = jwt.GetSigningMethod(configure.Config.GetString("jwt.algorithm"))
		}

		if algorithm == nil {
			algorithm = jwt.SigningMethodHS256
		}

		jwtMiddleware := jwtmiddleware.New(jwtmiddleware.Options{
			Extractor: jwtmiddleware.FromFirst(jwtmiddleware.FromAuthHeader, jwtmiddleware.FromParameter("jwt")),
			ValidationKeyGetter: func(token *jwt.Token) (interface{}, error) {
				return []byte(configure.Config.GetString("jwt.secret")), nil
			},
			SigningMethod: algorithm,
			ErrorHandler: func(w http.ResponseWriter, r *http.Request, err string) {
				res := &Response{
					w:      w,
					Status: 403,
					Data:   err,
				}
				res.SendJson()
			},
		})

		jwtMiddleware.HandlerWithNext(w, r, next.ServeHTTP)
	})
}

func (s *Server) Serve(l net.Listener) error {
	mux := http.NewServeMux()

	mux.Handle("/statics/", http.StripPrefix("/statics/", http.FileServer(http.Dir("statics"))))

	mux.HandleFunc("/stat/livestat", func(w http.ResponseWriter, r *http.Request) {
		s.GetLiveStatics(w, r)
	})
	http.Serve(l, JWTMiddleware(mux))
	return nil
}

type stream struct {
	Key             string `json:"key"`
	Url             string `json:"url"`
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

//http://127.0.0.1:8090/stat/livestat
func (server *Server) GetLiveStatics(w http.ResponseWriter, req *http.Request) {
	res := &Response{
		w:      w,
		Data:   nil,
		Status: 200,
	}

	defer res.SendJson()

	rtmpStream := server.handler.(*rtmp.RtmpStream)
	if rtmpStream == nil {
		res.Status = 500
		res.Data = "Get rtmp stream information error"
		return
	}

	msgs := new(streams)
	for item := range rtmpStream.GetStreams().IterBuffered() {
		if s, ok := item.Val.(*rtmp.Stream); ok {
			if s.GetReader() != nil {
				switch s.GetReader().(type) {
				case *rtmp.VirReader:
					v := s.GetReader().(*rtmp.VirReader)
					msg := stream{item.Key, v.Info().URL, v.ReadBWInfo.StreamId, v.ReadBWInfo.VideoDatainBytes, v.ReadBWInfo.VideoSpeedInBytesperMS,
						v.ReadBWInfo.AudioDatainBytes, v.ReadBWInfo.AudioSpeedInBytesperMS}
					msgs.Publishers = append(msgs.Publishers, msg)
				}
			}
		}
	}

	for item := range rtmpStream.GetStreams().IterBuffered() {
		ws := item.Val.(*rtmp.Stream).GetWs()
		for s := range ws.IterBuffered() {
			if pw, ok := s.Val.(*rtmp.PackWriterCloser); ok {
				if pw.GetWriter() != nil {
					switch pw.GetWriter().(type) {
					case *rtmp.VirWriter:
						v := pw.GetWriter().(*rtmp.VirWriter)
						msg := stream{item.Key, v.Info().URL, v.WriteBWInfo.StreamId, v.WriteBWInfo.VideoDatainBytes, v.WriteBWInfo.VideoSpeedInBytesperMS,
							v.WriteBWInfo.AudioDatainBytes, v.WriteBWInfo.AudioSpeedInBytesperMS}
						msgs.Players = append(msgs.Players, msg)
					}
				}
			}
		}
	}

	resp, _ := json.Marshal(msgs)
	res.Data = resp
}
