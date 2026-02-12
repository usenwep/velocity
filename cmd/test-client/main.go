package main

import (
	"fmt"
	"log"
	"os"
	"time"

	nwep "github.com/usenwep/nwep-go"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: test-client <web://url>\n")
		os.Exit(1)
	}
	url := os.Args[1]

	if err := nwep.Init(); err != nil {
		log.Fatal("init: ", err)
	}
	nwep.SetLogLevel(nwep.LogWarn)

	kp, err := nwep.GenerateKeypair()
	if err != nil {
		log.Fatal("keygen: ", err)
	}
	defer kp.Clear()

	client, err := nwep.NewClient(kp, nwep.WithOnNotify(func(n *nwep.Notification) {
		fmt.Printf("[notify] event=%s path=%s body=%s\n", n.Event, n.Path, string(n.Body))
	}))
	if err != nil {
		log.Fatal("client new: ", err)
	}

	fmt.Println("connecting to", url)
	if err := client.Connect(url); err != nil {
		log.Fatal("connect: ", err)
	}
	fmt.Println("connected!")

	// test get
	fmt.Println("\n- GET /health")
	resp, err := client.Get("/health")
	if err != nil {
		log.Fatal("get /health: ", err)
	}
	fmt.Printf("status=%s body=%s\n", resp.Status, string(resp.Body))

	// test notfound
	fmt.Println("\n- GET /nonexistent")
	resp, err = client.Get("/nonexistent")
	if err != nil {
		log.Fatal("get /nonexistent: ", err)
	}
	fmt.Printf("status=%s body=%s\n", resp.Status, string(resp.Body))

	time.Sleep(100 * time.Millisecond)

	fmt.Println("\n--- done ---")
	client.Close()
}
