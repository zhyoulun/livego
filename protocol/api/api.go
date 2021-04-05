package api

import (
	"encoding/json"
	"net"
	"net/http"
)

type Response struct {
	w      http.ResponseWriter
	Status int    `json:"status"`
	Data   []byte `json:"data"`
}

func (r *Response) SendJson() (int, error) {
	r.w.Header().Set("Content-Type", "application/json")
	r.w.WriteHeader(r.Status)
	return r.w.Write(r.Data)
}

type Server struct {
	handler  json.Marshaler
	rtmpAddr string
}

func NewServer(h json.Marshaler, rtmpAddr string) *Server {
	return &Server{
		handler:  h,
		rtmpAddr: rtmpAddr,
	}
}

func (s *Server) Serve(l net.Listener) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/stat/livestat", func(w http.ResponseWriter, r *http.Request) {
		s.GetLiveStatics(w, r)
	})
	return http.Serve(l, mux)
}

//http://127.0.0.1:8090/stat/livestat
func (s *Server) GetLiveStatics(w http.ResponseWriter, r *http.Request) {
	res := &Response{
		w:      w,
		Data:   nil,
		Status: 200,
	}

	defer res.SendJson()

	if s.handler == nil {
		res.Status = 500
		return
	}

	res.Data, _ = json.Marshal(s.handler)
}
