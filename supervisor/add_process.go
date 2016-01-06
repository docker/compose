package supervisor

type AddProcessEvent struct {
	s *Supervisor
}

// TODO: add this to worker for concurrent starts???  maybe not because of races where the container
// could be stopped and removed...
func (h *AddProcessEvent) Handle(e *Event) error {
	/*
		start := time.Now()
		ci, ok := h.s.containers[e.ID]
		if !ok {
			return ErrContainerNotFound
		}
			p, io, err := h.s.runtime.StartProcess(ci.container, *e.Process, e.Console)
			if err != nil {
				return err
			}
			if e.Pid, err = p.Pid(); err != nil {
				return err
			}
			h.s.processes[e.Pid] = &containerInfo{
				container: ci.container,
			}
			l, err := h.s.copyIO(e.Stdin, e.Stdout, e.Stderr, io)
			if err != nil {
				// log the error but continue with the other commands
				logrus.WithFields(logrus.Fields{
					"error": err,
					"id":    e.ID,
				}).Error("log stdio")
			}
			h.s.processes[e.Pid].copier = l
			ExecProcessTimer.UpdateSince(start)
	*/
	return nil
}
