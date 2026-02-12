package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	nwep "github.com/usenwep/nwep-go"
	"github.com/usenwep/velocity"
)

func main() {
	if err := nwep.Init(); err != nil {
		log.Fatal(err)
	}
	nwep.SetLogLevel(nwep.LogWarn)

	srv, err := velocity.New(":0",
		velocity.OnStart(func(s *velocity.Server) {
			fmt.Println("server listening on", s.Addr())
			fmt.Println("server url:", s.URL("/"))
		}),
	)
	if err != nil {
		log.Fatal(err)
	}

	srv.Handle("/ping", func(c *velocity.Context) error {
		return c.OK([]byte("pong"))
	})

	srv.Handle("/test-notify", func(c *velocity.Context) error {
		s := c.Server()
		peers := s.ConnectedPeers()
		if len(peers) == 0 {
			return c.Error(velocity.StatusBadRequest, "no peers")
		}
		peer := peers[0]

		// notif peer with small body
		fmt.Println("[server] test 1: Notify single peer, small body")
		if err := s.Notify(peer, "test1", "/small", []byte(`{"n":1}`)); err != nil {
			return c.Error(velocity.StatusInternalError, "test1: "+err.Error())
		}

		// notif all with nil body
		fmt.Println("[server] test 2: NotifyAll, nil body")
		s.NotifyAll("test2", "/nil-body", nil)

		// notif all but this time empty
		fmt.Println("[server] test 3: NotifyAll, empty body")
		s.NotifyAll("test3", "/empty-body", []byte{})

		// send notify to peer but with json
		fmt.Println("[server] test 4: NotifyJSON single peer")
		if err := s.NotifyJSON(peer, "test4", "/json", map[string]any{
			"key":    "value",
			"number": 42,
		}); err != nil {
			return c.Error(velocity.StatusInternalError, "test4: "+err.Error())
		}

		// notify all with json
		fmt.Println("[server] test 5: NotifyAllJSON")
		if err := s.NotifyAllJSON("test5", "/json-all", []string{"a", "b", "c"}); err != nil {
			return c.Error(velocity.StatusInternalError, "test5: "+err.Error())
		}

		// custom headers example
		fmt.Println("[server] test 6: NotifyWithOptions with headers")
		if err := s.NotifyWithOptions(peer, "test6", "/with-opts", []byte("opts-body"), &nwep.NotifyOptions{
			Headers: []nwep.Header{
				{Name: "x-custom", Value: "hello"},
				{Name: "x-another", Value: "world"},
			},
		}); err != nil {
			return c.Error(velocity.StatusInternalError, "test6: "+err.Error())
		}

		// rapid fire notifs
		fmt.Println("[server] test 8: rapid fire 50 notifications")
		for i := range 50 {
			body := fmt.Appendf(nil, `{"seq":%d}`, i)
			if err := s.Notify(peer, "test8", "/rapid", body); err != nil {
				return c.Error(velocity.StatusInternalError, fmt.Sprintf("test8 seq %d: %s", i, err))
			}
		}

		fmt.Println("[server] all notify tests sent")
		return c.OK([]byte("all tests sent"))
	})

	if err := srv.Start(); err != nil {
		log.Fatal(err)
	}
	go srv.NWEPServer().Run()

	// cli
	kp, err := nwep.GenerateKeypair()
	if err != nil {
		log.Fatal(err)
	}

	var mu sync.Mutex
	received := make(map[string]int) // event -> count
	var totalBytes int

	client, err := nwep.NewClient(kp, nwep.WithOnNotify(func(n *nwep.Notification) {
		mu.Lock()
		received[n.Event]++
		totalBytes += len(n.Body)
		mu.Unlock()
	}))
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("client connecting...")
	if err := client.Connect(srv.URL("/")); err != nil {
		log.Fatal("connect: ", err)
	}
	fmt.Println("client connected!")

	resp, err := client.Get("/ping")
	if err != nil {
		log.Fatal("ping: ", err)
	}
	fmt.Printf("ping: status=%s body=%s\n\n", resp.Status, string(resp.Body))

	resp, err = client.Get("/test-notify")
	if err != nil {
		log.Fatal("test-notify: ", err)
	}
	fmt.Printf("test-notify: status=%s body=%s\n\n", resp.Status, string(resp.Body))

	// waiting for notifs
	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	fmt.Println("- notification summary")
	total := 0
	for event, count := range received {
		fmt.Printf("  %-10s  received %d\n", event, count)
		total += count
	}
	fmt.Printf("\n  total notifications: %d\n", total)
	fmt.Printf("  total bytes received: %d\n", totalBytes)
	mu.Unlock()

	pass := true
	check := func(event string, expected int) {
		mu.Lock()
		got := received[event]
		mu.Unlock()
		if got != expected {
			fmt.Printf("  FAIL: %s expected %d, got %d\n", event, expected, got)
			pass = false
		}
	}
	check("test1", 1)
	check("test2", 1)
	// test3 has empty body
	check("test4", 1)
	check("test5", 1)
	check("test6", 1)
	check("test7", 1)
	check("test8", 50)

	if pass {
		fmt.Println("\n  ALL CHECKS PASSED")
	} else {
		fmt.Println("\n  SOME CHECKS FAILED")
	}

	client.Close()
	srv.Shutdown()
}
