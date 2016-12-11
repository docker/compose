package execution

type Supervisor struct {
}

type waiter interface {
	Wait() (uint32, error)
}

func (s *Supervisor) Monitor(w waiter, cb func(uint32, error)) {
	go cb(w.Wait())
}
