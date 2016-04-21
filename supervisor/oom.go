package supervisor

import (
	"time"

	"github.com/Sirupsen/logrus"
)

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
