package valclient

import (
	"sync"
	"testing"
)

func TestClientTokenConcurrency(t *testing.T) {
	c := NewClient("https://val.rmit.edu.au", "initial_token")

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(2)
		go func(idx int) {
			defer wg.Done()
			c.SetToken("updated_token")
		}(i)

		go func() {
			defer wg.Done()
			_ = c.GetToken()
		}()
	}
	wg.Wait()
}
