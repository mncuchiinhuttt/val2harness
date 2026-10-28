package analytics

func (t *Tracker) Subscribe() chan AnalyticsSummary {
	t.mu.Lock()
	defer t.mu.Unlock()
	ch := make(chan AnalyticsSummary, 1)
	// Bound subscriptions as well as stored observations. A closed channel lets
	// an excess consumer terminate without retaining another listener.
	if len(t.listeners) >= 128 {
		close(ch)
		return ch
	}
	t.listeners[ch] = struct{}{}
	return ch
}

func (t *Tracker) Unsubscribe(ch chan AnalyticsSummary) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.listeners[ch]; ok {
		delete(t.listeners, ch)
		close(ch)
	}
}

// broadcast runs under mu so reset and updates cannot be reordered. Each
// subscriber receives an independent copy and keeps the latest snapshot.
func (t *Tracker) broadcast() {
	for ch := range t.listeners {
		select {
		case <-ch:
		default:
		}
		ch <- t.buildSummary()
	}
}
