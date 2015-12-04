package v1

import (
	"encoding/json"
	"net/http"
	"strconv"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/docker/containerd"
	"github.com/docker/containerd/runtime"
	"github.com/gorilla/mux"
	"github.com/opencontainers/specs"
)

func NewServer(supervisor *containerd.Supervisor) http.Handler {
	r := mux.NewRouter()
	s := &server{
		supervisor: supervisor,
		r:          r,
	}
	// process handlers
	r.HandleFunc("/containers/{id:.*}/process/{pid:.*}", s.signalPid).Methods("POST")
	r.HandleFunc("/containers/{id:.*}/process", s.addProcess).Methods("PUT")

	// checkpoint and restore handlers
	// TODO: PUT handler for adding a checkpoint to containerd??
	r.HandleFunc("/containers/{id:.*}/checkpoint/{name:.*}", s.createCheckpoint).Methods("POST")
	// r.HandleFunc("/containers/{id:.*}/checkpoint/{cid:.*}", s.deleteCheckpoint).Methods("DELETE")
	r.HandleFunc("/containers/{id:.*}/checkpoint", s.listCheckpoints).Methods("GET")

	// container handlers
	r.HandleFunc("/containers/{id:.*}", s.createContainer).Methods("POST")
	r.HandleFunc("/containers/{id:.*}", s.updateContainer).Methods("PATCH")

	// internal method for replaying the journal
	r.HandleFunc("/event", s.event).Methods("POST")
	r.HandleFunc("/events", s.events).Methods("GET")

	// containerd handlers
	r.HandleFunc("/state", s.state).Methods("GET")
	return s
}

type server struct {
	r          *mux.Router
	supervisor *containerd.Supervisor
}

// TODO: implement correct shutdown
func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.r.ServeHTTP(w, r)
}

func (s *server) updateContainer(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var state ContainerState
	if err := json.NewDecoder(r.Body).Decode(&state); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	e := containerd.NewEvent(containerd.UpdateContainerEventType)
	e.ID = id
	e.State = &runtime.State{
		Status: runtime.Status(string(state.Status)),
	}
	s.supervisor.SendEvent(e)
	if err := <-e.Err; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *server) events(w http.ResponseWriter, r *http.Request) {
	events := s.supervisor.Events()
	enc := json.NewEncoder(w)
	for evt := range events {
		var v interface{}
		switch evt.Type {
		case containerd.ExitEventType:
			v = createExitEvent(evt)
		}
		if err := enc.Encode(v); err != nil {
			// TODO: handled closed conn
			logrus.WithField("error", err).Error("encode event")
		}
	}
}

func createExitEvent(e *containerd.Event) *Event {
	return &Event{
		Type:   "exit",
		ID:     e.ID,
		Status: e.Status,
	}
}

func (s *server) event(w http.ResponseWriter, r *http.Request) {
	var e containerd.Event
	if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	e.Err = make(chan error, 1)
	s.supervisor.SendEvent(&e)
	if err := <-e.Err; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if e.Containers != nil && len(e.Containers) > 0 {
		if err := s.writeState(w, &e); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func (s *server) addProcess(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var process specs.Process
	if err := json.NewDecoder(r.Body).Decode(&process); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	e := containerd.NewEvent(containerd.AddProcessEventType)
	e.ID = id
	e.Process = &process
	s.supervisor.SendEvent(e)
	if err := <-e.Err; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	p := Process{
		Pid:      e.Pid,
		Terminal: process.Terminal,
		Args:     process.Args,
		Env:      process.Env,
		Cwd:      process.Cwd,
	}
	p.User.UID = process.User.UID
	p.User.GID = process.User.GID
	p.User.AdditionalGids = process.User.AdditionalGids
	if err := json.NewEncoder(w).Encode(p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
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

func (s *server) state(w http.ResponseWriter, r *http.Request) {
	e := containerd.NewEvent(containerd.GetContainerEventType)
	s.supervisor.SendEvent(e)
	if err := <-e.Err; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.writeState(w, e); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *server) writeState(w http.ResponseWriter, e *containerd.Event) error {
	m := s.supervisor.Machine()
	state := State{
		Containers: []Container{},
		Machine: Machine{
			Cpus:   m.Cpus,
			Memory: m.Memory,
		},
	}
	for _, c := range e.Containers {
		processes, err := c.Processes()
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"error":     err,
				"container": c.ID(),
			}).Error("get processes for container")
		}
		var pids []Process
		for _, p := range processes {
			if proc := createProcess(p); proc != nil {
				pids = append(pids, *proc)
			}
		}
		state.Containers = append(state.Containers, Container{
			ID:         c.ID(),
			BundlePath: c.Path(),
			Processes:  pids,
			State: &ContainerState{
				Status: Status(c.State().Status),
			},
		})
	}
	return json.NewEncoder(w).Encode(&state)
}

func createProcess(in runtime.Process) *Process {
	pid, err := in.Pid()
	if err != nil {
		logrus.WithField("error", err).Error("get process pid")
		return nil
	}
	process := in.Spec()
	p := &Process{
		Pid:      pid,
		Terminal: process.Terminal,
		Args:     process.Args,
		Env:      process.Env,
		Cwd:      process.Cwd,
	}
	p.User.UID = process.User.UID
	p.User.GID = process.User.GID
	p.User.AdditionalGids = process.User.AdditionalGids
	return p
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
	if c.Checkpoint != nil {
		e.Checkpoint = &runtime.Checkpoint{
			Name: c.Checkpoint.Name,
			Path: c.Checkpoint.Path,
		}
	}
	e.Stdio = &runtime.Stdio{
		Stderr: c.Stderr,
		Stdout: c.Stdout,
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

func (s *server) listCheckpoints(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	e := containerd.NewEvent(containerd.GetContainerEventType)
	s.supervisor.SendEvent(e)
	if err := <-e.Err; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var container runtime.Container
	for _, c := range e.Containers {
		if c.ID() == id {
			container = c
			break
		}
	}
	if container == nil {
		http.Error(w, "container not found", http.StatusNotFound)
		return
	}
	checkpoints, err := container.Checkpoints()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := []Checkpoint{}
	for _, c := range checkpoints {
		out = append(out, Checkpoint{
			Path:        c.Path,
			Name:        c.Name,
			Tcp:         c.Tcp,
			Shell:       c.Shell,
			UnixSockets: c.UnixSockets,
		})
	}
}

func (s *server) createCheckpoint(w http.ResponseWriter, r *http.Request) {
	var (
		vars = mux.Vars(r)
		id   = vars["id"]
		name = vars["name"]
	)
	var cp Checkpoint
	if err := json.NewDecoder(r.Body).Decode(&cp); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	e := containerd.NewEvent(containerd.CreateCheckpointEventType)
	e.ID = id
	e.Checkpoint = &runtime.Checkpoint{
		Name:        name,
		Path:        cp.Path,
		Running:     cp.Running,
		Tcp:         cp.Tcp,
		UnixSockets: cp.UnixSockets,
		Shell:       cp.Shell,
	}
	s.supervisor.SendEvent(e)
	if err := <-e.Err; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (s *server) deleteCheckpoint(w http.ResponseWriter, r *http.Request) {
}
