package supervisor

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/containerd/runtime"
)

type containerInfo struct {
	container runtime.Container
}

func setupEventLog(s *Supervisor, retainCount int) error {
	if err := readEventLog(s); err != nil {
		return err
	}
	logrus.WithField("count", len(s.eventLog)).Debug("containerd: read past events")
	events := s.Events(time.Time{}, false, "")
	return eventLogger(s, filepath.Join(s.config.StateDir, "events.log"), events, retainCount)
}

func eventLogger(s *Supervisor, path string, events chan Event, retainCount int) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	go func() {
		var (
			count = len(s.eventLog)
			enc   = json.NewEncoder(f)
		)
		for e := range events {
			// if we have a specified retain count make sure the truncate the event
			// log if it grows past the specified number of events to keep.
			if retainCount > 0 {
				if count > retainCount {
					logrus.Debug("truncating event log")
					// close the log file
					if f != nil {
						f.Close()
					}
					slice := retainCount - 1
					l := len(s.eventLog)
					if slice >= l {
						slice = l
					}
					s.eventLock.Lock()
					s.eventLog = s.eventLog[len(s.eventLog)-slice:]
					s.eventLock.Unlock()
					if f, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND|os.O_TRUNC, 0755); err != nil {
						logrus.WithField("error", err).Error("containerd: open event to journal")
						continue
					}
					enc = json.NewEncoder(f)
					count = 0
					for _, le := range s.eventLog {
						if err := enc.Encode(le); err != nil {
							logrus.WithField("error", err).Error("containerd: write event to journal")
						}
					}
				}
			}
			s.eventLock.Lock()
			s.eventLog = append(s.eventLog, e)
			s.eventLock.Unlock()
			count++
			if err := enc.Encode(e); err != nil {
				logrus.WithField("error", err).Error("containerd: write event to journal")
			}
		}
	}()
	return nil
}

func readEventLog(s *Supervisor) error {
	f, err := os.Open(filepath.Join(s.config.StateDir, "events.log"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	for {
		var e Event
		if err := dec.Decode(&e); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		s.eventLog = append(s.eventLog, e)
	}
	return nil
}
