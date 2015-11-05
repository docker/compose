package v1

import (
	"encoding/json"
	"net/http"

	"github.com/crosbymichael/containerd"
	"github.com/gorilla/mux"
)

func NewServer(supervisor *containerd.Supervisor) http.Handler {
	r := mux.NewRouter()
	s := &server{
		supervisor: supervisor,
		r:          r,
	}
	r.HandleFunc("/containers", s.containers).Methods("GET")
	r.HandleFunc("/containers/{id:*}", s.createContainer).Methods("POST")
	r.HandleFunc("/containers/{id:*}", s.deleteContainer).Methods("DELETE")
	return s
}

type server struct {
	r          *mux.Router
	supervisor *containerd.Supervisor
}

func (s *server) HandleHTTP(w http.ResponseWriter, r *http.Request) {
	s.r.HandleHTTP(w, r)
}

func (s *server) containers(w http.ResponseWriter, r *http.Request) {

}

func (s *server) events(w http.ResponseWriter, r *http.Request) {

}

func (s *server) deleteContainer(w http.ResponseWriter, r *http.Request) {

}

func (s *server) createContainer(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var c Container
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	e := &containerd.CreateContainerEvent{
		ID:         id,
		BundlePath: c.BundlePath,
		Err:        make(chan error, 1),
	}
	s.supervisor.SendEvent(e)
	if err := <-e.Err; err != nil {
		code := http.StatusInternalServerError
		if err == containerd.ErrBundleNotFound {
			code = http.StatusNotFound
		}
		http.Error(w, err.Error(), code)
		return
	}
	w.WriteHeader(http.StatusCreated)
}
