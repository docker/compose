package supervisor

import (
	"time"

	"github.com/Sirupsen/logrus"
)

// OOMTask holds needed parameters to report a container OOM
type OOMTask struct {
	baseTask
	ID string
}

func (s *Supervisor) oom(t *OOMTask) error {
	logrus.WithField("id", t.ID).Debug("containerd: container oom")
	s.notifySubscribers(Event{
		Timestamp: time.Now(),
		ID:        t.ID,
		Type:      StateOOM,
	})
	return nil
}
