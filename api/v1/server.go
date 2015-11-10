package v1

import (
	"encoding/json"
	"net/http"
	"strconv"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/crosbymichael/containerd"
	"github.com/gorilla/mux"
)

func NewServer(supervisor *containerd.Supervisor) http.Handler {
	r := mux.NewRouter()
	s := &server{
		supervisor: supervisor,
		r:          r,
	}
	r.HandleFunc("/containers/{id:.*}/process/{pid:.*}", s.signalPid).Methods("POST")
	r.HandleFunc("/containers/{id:.*}", s.createContainer).Methods("POST")
	r.HandleFunc("/containers", s.containers).Methods("GET")
	//	r.HandleFunc("/containers/{id:.*}", s.deleteContainer).Methods("DELETE")
	return s
}

type server struct {
	r          *mux.Router
	supervisor *containerd.Supervisor
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.r.ServeHTTP(w, r)
}

func (s *server) signalPid(w http.ResponseWriter, r *http.Request) {
	var (
		vars = mux.Vars(r)
		id   = vars["id"]
		spid = vars["pid"]
	)
	pid, err := strconv.Atoi(spid)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var signal Signal
	if err := json.NewDecoder(r.Body).Decode(&signal); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	e := containerd.NewEvent(containerd.SignalEventType)
	e.ID = id
	e.Pid = pid
	e.Signal = syscall.Signal(signal.Signal)
	s.supervisor.SendEvent(e)
	if err := <-e.Err; err != nil {
		status := http.StatusInternalServerError
		if err == containerd.ErrContainerNotFound {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
}

func (s *server) containers(w http.ResponseWriter, r *http.Request) {
	var state State
	state.Containers = []Container{}
	e := containerd.NewEvent(containerd.GetContainerEventType)
	s.supervisor.SendEvent(e)
	if err := <-e.Err; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, c := range e.Containers {
		processes, err := c.Processes()
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"error":     err,
				"container": c.ID(),
			}).Error("get processes for container")
		}
		var pids []int
		for _, p := range processes {
			pids = append(pids, p.Pid())
		}
		state.Containers = append(state.Containers, Container{
			ID:         c.ID(),
			BundlePath: c.Path(),
			Processes:  pids,
		})
	}
	if err := json.NewEncoder(w).Encode(&state); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *server) createContainer(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var c Container
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if c.BundlePath == "" {
		http.Error(w, "empty bundle path", http.StatusBadRequest)
		return
	}
	e := containerd.NewEvent(containerd.StartContainerEventType)
	e.ID = id
	e.BundlePath = c.BundlePath
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
