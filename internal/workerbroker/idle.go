package workerbroker

import "errors"

// QuiesceIdle atomically proves that all recorded runs and their cleanup are
// finished, then prevents admission through existing clients before shutdown.
func (b *Broker) QuiesceIdle() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.persistenceErr != nil {
		return errors.New("worker state needs recovery before settings can change")
	}
	if len(b.active) > 0 {
		return errors.New("wait for active worker execution and cleanup before changing settings")
	}
	for _, run := range b.state.Runs {
		if run.Run.Status != "completed" && run.Run.Status != "cancelled" {
			return errors.New("finish or cancel recorded worker runs before changing settings")
		}
	}
	b.quiesced = true
	return nil
}
