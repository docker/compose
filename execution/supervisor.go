package execution

type Supervisor struct {
}

type waiter interface {
	Wait() (uint32, error)
}

func (s *Supervisor) Add(w waiter) {

}
