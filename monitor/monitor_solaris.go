package monitor

import "errors"

func New() (*Monitor, error) {
	return nil, errors.New("Monitor NewMonitor() not implemented on Solaris")
}

type Monitor struct {
}

func (m *Monitor) Events() chan Monitorable {
	return nil
}

func (m *Monitor) Add(Monitorable) error {
	return errors.New("Monitor Add() not implemented on Solaris")
}

func (m *Monitor) Remove(Monitorable) error {
	return errors.New("Monitor Remove() not implemented on Solaris")
}

func (m *Monitor) Close() error {
	return errors.New("Monitor Close() not implemented on Solaris")
}

func (m *Monitor) Run() {
}
