package analytics

func (t *Tracker) Subscribe() chan AnalyticsSummary {
	t.listenersMu.Lock()
	defer t.listenersMu.Unlock()
	ch := make(chan AnalyticsSummary, 10)
	t.listeners[ch] = struct{}{}
	return ch
}

func (t *Tracker) Unsubscribe(ch chan AnalyticsSummary) {
	t.listenersMu.Lock()
	defer t.listenersMu.Unlock()
	delete(t.listeners, ch)
	close(ch)
}

func (t *Tracker) broadcast(summary AnalyticsSummary) {
	t.listenersMu.Lock()
	defer t.listenersMu.Unlock()
	for ch := range t.listeners {
		select {
		case ch <- summary:
		default:
		}
	}
}
