package managedworkers

import "errors"

// ResetIdle retires an idle broker so its next connection adopts new settings.
// Durable runs, artifacts, the environment manifest and login remain intact.
func (m *Manager) ResetIdle(projectID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errors.New("worker manager is stopped")
	}
	r := m.running[projectID]
	if r == nil {
		return nil
	}
	idle, ok := r.broker.(interface{ QuiesceIdle() error })
	if !ok {
		return errors.New("this worker runtime cannot safely verify idle status")
	}
	if err := idle.QuiesceIdle(); err != nil {
		return err
	}
	_ = r.server.Close()
	r.cancel()
	<-r.done
	err := errors.Join(r.err, r.broker.Close())
	delete(m.running, projectID)
	return err
}
