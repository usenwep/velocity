package main

import (
	"log"

	"github.com/usenwep/velocity"
)

func main() {
	srv, err := velocity.New(":6937",
		velocity.WithKeyFile("server.key"),
	)
	if err != nil {
		log.Fatal(err)
	}

	srv.Use(velocity.Recover(), velocity.RequestLogger())

	srv.Handle("/echo", func(c *velocity.Context) error {
		return c.OK(c.Body())
	})

	if err := srv.Run(); err != nil {
		log.Fatal(err)
	}
}
