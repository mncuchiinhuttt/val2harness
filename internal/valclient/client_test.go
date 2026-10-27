package valclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestListModelsEmptyAndMalformedResponses(t *testing.T) {
	for _, tc := range []struct {
		body  string
		valid bool
	}{
		{`{"data":[]}`, true}, {`[]`, true}, {`{"data":[{"id":"synthetic"}]}`, true},
		{`null`, false}, {`{"data":null}`, false}, {`{}`, false}, {`{"data":{}}`, false},
	} {
		t.Run(tc.body, func(t *testing.T) {
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, tc.body) }))
			defer up.Close()
			models, err := NewClient(up.URL, "synthetic").ListModels(context.Background())
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v, models=%v error=%v", tc.valid, models, err)
			}
		})
	}
}

func TestConcurrentTokenRefreshAcrossAllRequests(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if key := r.Header.Get("Authorization"); key != "Bearer synthetic-a" && key != "Bearer synthetic-b" {
			t.Errorf("invalid credential snapshot: %q", key)
		}
		switch r.URL.Path {
		case "/api/models":
			fmt.Fprint(w, `{"data":[{"id":"synthetic"}]}`)
		case "/api/v1/chats/":
			fmt.Fprint(w, `[]`)
		default:
			fmt.Fprint(w, `{}`)
		}
	}))
	defer up.Close()
	c := NewClient(up.URL, "synthetic-a")
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 2000; i++ {
			if i%2 == 0 {
				c.SetToken("synthetic-a")
			} else {
				c.SetToken("synthetic-b")
			}
		}
	}()
	for operation := 0; operation < 5; operation++ {
		wg.Add(1)
		go func(op int) {
			defer wg.Done()
			<-start
			for i := 0; i < 30; i++ {
				var err error
				switch op {
				case 0:
					if key := c.GetToken(); key != "synthetic-a" && key != "synthetic-b" {
						t.Error("invalid token snapshot")
					}
				case 1:
					_, err = c.ListModels(context.Background())
				case 2:
					_, err = c.ListChats(context.Background())
				case 3:
					_, err = c.GetChat(context.Background(), "synthetic-chat")
				case 4:
					var resp *http.Response
					resp, err = c.ProxyChat(context.Background(), strings.NewReader(`{}`))
					if err == nil {
						io.Copy(io.Discard, resp.Body)
						resp.Body.Close()
					}
				}
				if err != nil {
					t.Error(err)
				}
			}
		}(operation)
	}
	close(start)
	wg.Wait()
}
