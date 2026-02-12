package velocity_test

import (
	"github.com/usenwep/velocity"

	nwep "github.com/usenwep/nwep-go"
)

// This is a build-only test / compile check - it verifies that the velocity API compiles
// correctly. It cannot run without the nwep C library at runtime.

var _ = func() {
	srv, _ := velocity.New(":6937",
		velocity.WithSettings(nwep.Settings{MaxStreams: 200}),
		velocity.WithLogger(velocity.DefaultLogger()),
		velocity.OnStart(func(s *velocity.Server) {}),
		velocity.OnShutdown(func(s *velocity.Server) {}),
	)

	srv.Use(velocity.Recover(), velocity.RequestLogger())

	srv.Handle("/hello", func(c *velocity.Context) error {
		return c.OK([]byte("hello from velocity"))
	})

	api := srv.Group("/api/v1")
	api.Read("/items", func(c *velocity.Context) error {
		return c.JSON(map[string]string{"status": "ok"})
	})
	api.Write("/items", func(c *velocity.Context) error {
		var body map[string]any
		if err := c.Bind(&body); err != nil {
			return c.BadRequest(err.Error())
		}
		return c.Created(nil)
	})

	srv.Handle("/echo", func(c *velocity.Context) error {
		_ = c.Method()
		_ = c.Path()
		_ = c.Body()
		_ = c.RequestID()
		_ = c.TraceID()
		_ = c.PeerNodeID()
		_ = c.Conn()
		c.Set("key", "value")
		_ = c.MustGet("key")
		_ = c.Logger()
		_ = c.Server()
		return c.NoContent()
	})

	_ = srv.Router()
	_ = srv.NodeID()

	_ = velocity.MustKeypair(nwep.GenerateKeypair())

	var peer nwep.NodeID
	_ = srv.Notify(peer, "update", "/data", []byte("{}"))
	_ = srv.NotifyJSON(peer, "update", "/data", map[string]string{"a": "b"})
	srv.NotifyAll("update", "/data", nil)
	_ = srv.NotifyAllJSON("update", "/data", nil)
	_ = srv.ConnectionCount()
	_ = srv.ConnectedPeers()

	_ = velocity.RequirePeer()
	_ = velocity.AllowPeers(peer)
	_ = velocity.MethodFilter(velocity.MethodRead, velocity.MethodWrite)

	_ = velocity.StatusOK
	_ = velocity.StatusNotFound
	_ = velocity.MethodRead

	tc := &velocity.TrustConfig{}
	_, _ = tc.Build()

	cfg := velocity.DefaultConfig()
	_ = cfg

	// compile check for log and anchor
	_ = velocity.WithLogServer(nil)
	_ = velocity.WithAnchorServer(nil)
	_ = srv.LogServer()
	_ = srv.AnchorServer()

	// lifecycle
	_ = srv
}
