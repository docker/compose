package v1

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

func NewClient(addr string) *Client {
	return &Client{
		addr: addr,
	}
}

type Client struct {
	addr string
}

type StartOpts struct {
	Path       string
	Checkpoint string
}

// Start starts a container with the specified id and path to the container's
// bundle on the system.
func (c *Client) Start(id string, opts StartOpts) error {
	container := Container{
		BundlePath: opts.Path,
		Checkpoint: opts.Checkpoint,
	}
	buf := bytes.NewBuffer(nil)
	if err := json.NewEncoder(buf).Encode(container); err != nil {
		return err
	}
	r, err := http.Post(c.addr+"/containers/"+id, "application/json", buf)
	if err != nil {
		return err
	}
	r.Body.Close()
	if r.StatusCode != http.StatusCreated {
		return fmt.Errorf("unexpected status %d", r.StatusCode)
	}
	return nil
}

func (c *Client) State() ([]Container, error) {
	r, err := http.Get(c.addr + "/state")
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		return nil, err
	}
	r.Body.Close()
	return s.Containers, nil
}

func (c *Client) SignalProcess(id string, pid, signal int) error {
	sig := Signal{
		Signal: signal,
	}
	buf := bytes.NewBuffer(nil)
	if err := json.NewEncoder(buf).Encode(sig); err != nil {
		return err
	}
	r, err := http.Post(c.addr+"/containers/"+id+"/process/"+strconv.Itoa(pid), "application/json", buf)
	if err != nil {
		return err
	}
	r.Body.Close()
	return nil
}

func (c *Client) Checkpoints(id string) ([]Checkpoint, error) {
	r, err := http.Get(c.addr + "/containers/" + id + "/checkpoint")
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	var checkpoints []Checkpoint
	if err := json.NewDecoder(r.Body).Decode(&checkpoints); err != nil {
		return nil, err
	}
	return checkpoints, nil
}

func (c *Client) CreateCheckpoint(id, name string, cp Checkpoint) error {
	buf := bytes.NewBuffer(nil)
	if err := json.NewEncoder(buf).Encode(cp); err != nil {
		return err
	}
	r, err := http.Post(c.addr+"/containers/"+id+"/checkpoint", "application/json", buf)
	if err != nil {
		return err
	}
	r.Body.Close()
	return nil
}
