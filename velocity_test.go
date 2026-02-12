package velocity

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	nwep "github.com/usenwep/nwep-go"
)

func init() {
	if err := nwep.Init(); err != nil {
		panic("nwep.Init: " + err.Error())
	}
	nwep.SetLogLevel(nwep.LogWarn)
}

// memLogStorage is an in-memory LogStorage for testing.
type memLogStorage struct {
	mu      sync.Mutex
	entries [][]byte
}

func (s *memLogStorage) Append(index uint64, entry []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if int(index) != len(s.entries) {
		return fmt.Errorf("index mismatch: %d vs %d", index, len(s.entries))
	}
	s.entries = append(s.entries, append([]byte(nil), entry...))
	return nil
}

func (s *memLogStorage) Get(index uint64, buf []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if int(index) >= len(s.entries) {
		return -1, fmt.Errorf("index out of range")
	}
	n := copy(buf, s.entries[int(index)])
	return n, nil
}

func (s *memLogStorage) Size() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return uint64(len(s.entries))
}

func makeTestEntry(t *testing.T) *nwep.MerkleEntry {
	t.Helper()
	kp, err := nwep.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	defer kp.Clear()

	nid, err := kp.NodeID()
	if err != nil {
		t.Fatal(err)
	}

	entry := &nwep.MerkleEntry{
		Type:      nwep.LogEntryKeyBinding,
		Timestamp: uint64(time.Now().UnixNano()),
		NodeID:    nid,
		Pubkey:    kp.PublicKey(),
	}

	encoded, err := nwep.MerkleEntryEncode(entry)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := nwep.Sign(kp, encoded)
	if err != nil {
		t.Fatal(err)
	}
	entry.Signature = sig
	return entry
}

// startTestServer creates a velocity server with the given options, starts it,
// and returns it along with a connected client. The caller should defer shutdown.
func startTestServer(t *testing.T, opts ...Option) (*Server, *nwep.Client) {
	t.Helper()

	srv, err := New(":0", opts...)
	if err != nil {
		t.Fatal("velocity.New:", err)
	}
	if err := srv.Start(); err != nil {
		t.Fatal("Start:", err)
	}

	go srv.NWEPServer().Run()

	time.Sleep(50 * time.Millisecond)

	clientKP, err := nwep.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { clientKP.Clear() })

	client, err := nwep.NewClient(clientKP, nwep.WithClientSettings(nwep.Settings{TimeoutMs: 5000}))
	if err != nil {
		t.Fatal(err)
	}

	url := srv.URL("/")
	if err := client.Connect(url); err != nil {
		t.Fatal("connect:", err)
	}

	return srv, client
}

func TestVelocityWithLogServer(t *testing.T) {
	storage := &memLogStorage{}
	ml, err := nwep.NewMerkleLog(storage)
	if err != nil {
		t.Fatal(err)
	}
	defer ml.Free()

	entry := makeTestEntry(t)
	ml.Append(entry)

	ls, err := nwep.NewLogServer(ml, nil)
	if err != nil {
		t.Fatal(err)
	}
	// ownership transfers to velocity; freed in Shutdown
	srv, client := startTestServer(t, WithLogServer(ls))
	defer func() {
		client.Close()
		srv.Shutdown()
	}()

	if srv.LogServer() != ls {
		t.Fatal("LogServer() should return set instance")
	}
	if srv.AnchorServer() != nil {
		t.Fatal("AnchorServer() should be nil")
	}

	t.Run("GET /log/size", func(t *testing.T) {
		resp, err := client.Get("/log/size")
		if err != nil {
			t.Fatal(err)
		}
		if resp.Status != "ok" {
			t.Fatalf("status = %q, want ok", resp.Status)
		}
		var body map[string]int
		if err := json.Unmarshal(resp.Body, &body); err != nil {
			t.Fatal("unmarshal:", err)
		}
		if body["size"] != 1 {
			t.Fatalf("size = %d, want 1", body["size"])
		}
	})

	t.Run("GET /log/entry/0", func(t *testing.T) {
		resp, err := client.Get("/log/entry/0")
		if err != nil {
			t.Fatal(err)
		}
		if resp.Status != "ok" {
			t.Fatalf("status = %q", resp.Status)
		}
		decoded, err := nwep.MerkleEntryDecode(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if decoded.NodeID != entry.NodeID {
			t.Fatal("entry NodeID mismatch")
		}
	})

	t.Run("GET /log/proof/0", func(t *testing.T) {
		resp, err := client.Get("/log/proof/0")
		if err != nil {
			t.Fatal(err)
		}
		if resp.Status != "ok" {
			t.Fatalf("status = %q", resp.Status)
		}
		if len(resp.Body) == 0 {
			t.Fatal("empty proof body")
		}
	})

	t.Run("GET /hello -> not_found", func(t *testing.T) {
		resp, err := client.Get("/hello")
		if err != nil {
			t.Fatal(err)
		}
		if resp.Status != "not_found" {
			t.Fatalf("status = %q, want not_found", resp.Status)
		}
	})
}

func TestVelocityWithLogServerAndRoutes(t *testing.T) {
	storage := &memLogStorage{}
	ml, err := nwep.NewMerkleLog(storage)
	if err != nil {
		t.Fatal(err)
	}
	defer ml.Free()

	entry := makeTestEntry(t)
	ml.Append(entry)

	ls, err := nwep.NewLogServer(ml, nil)
	if err != nil {
		t.Fatal(err)
	}

	srv, client := startTestServer(t, WithLogServer(ls))
	defer func() {
		client.Close()
		srv.Shutdown()
	}()

	srv.Handle("/hello", func(c *Context) error {
		return c.OK([]byte("hello from velocity"))
	})

	t.Run("log/size", func(t *testing.T) {
		resp, err := client.Get("/log/size")
		if err != nil {
			t.Fatal(err)
		}
		if resp.Status != "ok" {
			t.Fatalf("status = %q", resp.Status)
		}
	})

	t.Run("hello", func(t *testing.T) {
		resp, err := client.Get("/hello")
		if err != nil {
			t.Fatal(err)
		}
		if resp.Status != "ok" || string(resp.Body) != "hello from velocity" {
			t.Fatalf("status=%q body=%q", resp.Status, resp.Body)
		}
	})

	t.Run("unknown", func(t *testing.T) {
		resp, err := client.Get("/unknown")
		if err != nil {
			t.Fatal(err)
		}
		if resp.Status != "not_found" {
			t.Fatalf("status = %q", resp.Status)
		}
	})
}

func TestVelocityWithAnchorServer(t *testing.T) {
	blsKP, err := nwep.BLSKeypairGenerate()
	if err != nil {
		t.Fatal(err)
	}

	anchorSet, err := nwep.NewAnchorSet(1)
	if err != nil {
		t.Fatal(err)
	}
	defer anchorSet.Free()
	anchorSet.Add(blsKP.Pubkey(), true)

	as, err := nwep.NewAnchorServer(blsKP, anchorSet, nil)
	if err != nil {
		t.Fatal(err)
	}

	logStorage := &memLogStorage{}
	ml, err := nwep.NewMerkleLog(logStorage)
	if err != nil {
		t.Fatal(err)
	}
	defer ml.Free()
	ml.Append(makeTestEntry(t))
	root, _ := ml.Root()

	cp, err := nwep.CheckpointNew(1, uint64(time.Now().UnixNano()), root, ml.Size())
	if err != nil {
		t.Fatal(err)
	}
	nwep.CheckpointSign(cp, blsKP)
	as.AddCheckpoint(cp)

	srv, client := startTestServer(t, WithAnchorServer(as))
	defer func() {
		client.Close()
		srv.Shutdown()
	}()

	if srv.AnchorServer() != as {
		t.Fatal("AnchorServer() mismatch")
	}

	t.Run("checkpoint/latest", func(t *testing.T) {
		resp, err := client.Get("/checkpoint/latest")
		if err != nil {
			t.Fatal(err)
		}
		if resp.Status != "ok" {
			t.Fatalf("status = %q", resp.Status)
		}
		decoded, err := nwep.CheckpointDecode(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if decoded.Epoch != 1 {
			t.Fatalf("epoch = %d, want 1", decoded.Epoch)
		}
	})

	t.Run("checkpoint/1", func(t *testing.T) {
		resp, err := client.Get("/checkpoint/1")
		if err != nil {
			t.Fatal(err)
		}
		if resp.Status != "ok" {
			t.Fatalf("status = %q", resp.Status)
		}
	})

	t.Run("unknown -> not_found", func(t *testing.T) {
		resp, err := client.Get("/status")
		if err != nil {
			t.Fatal(err)
		}
		if resp.Status != "not_found" {
			t.Fatalf("status = %q", resp.Status)
		}
	})
}

func TestVelocityWithBothLogAndAnchorServers(t *testing.T) {
	logStorage := &memLogStorage{}
	ml, err := nwep.NewMerkleLog(logStorage)
	if err != nil {
		t.Fatal(err)
	}
	defer ml.Free()
	ml.Append(makeTestEntry(t))

	ls, err := nwep.NewLogServer(ml, nil)
	if err != nil {
		t.Fatal(err)
	}

	blsKP, err := nwep.BLSKeypairGenerate()
	if err != nil {
		t.Fatal(err)
	}
	anchorSet, err := nwep.NewAnchorSet(1)
	if err != nil {
		t.Fatal(err)
	}
	defer anchorSet.Free()
	anchorSet.Add(blsKP.Pubkey(), true)

	as, err := nwep.NewAnchorServer(blsKP, anchorSet, nil)
	if err != nil {
		t.Fatal(err)
	}

	root, _ := ml.Root()
	cp, _ := nwep.CheckpointNew(1, uint64(time.Now().UnixNano()), root, ml.Size())
	nwep.CheckpointSign(cp, blsKP)
	as.AddCheckpoint(cp)

	srv, client := startTestServer(t,
		WithLogServer(ls),
		WithAnchorServer(as),
		WithRole("log_server"),
	)
	defer func() {
		client.Close()
		srv.Shutdown()
	}()

	srv.Handle("/api/health", func(c *Context) error {
		return c.OK([]byte("ok"))
	})

	t.Run("log/size", func(t *testing.T) {
		resp, err := client.Get("/log/size")
		if err != nil {
			t.Fatal(err)
		}
		if resp.Status != "ok" {
			t.Fatalf("status = %q", resp.Status)
		}
	})

	t.Run("log/entry/0", func(t *testing.T) {
		resp, err := client.Get("/log/entry/0")
		if err != nil {
			t.Fatal(err)
		}
		if resp.Status != "ok" {
			t.Fatalf("status = %q", resp.Status)
		}
	})

	t.Run("checkpoint/latest", func(t *testing.T) {
		resp, err := client.Get("/checkpoint/latest")
		if err != nil {
			t.Fatal(err)
		}
		if resp.Status != "ok" {
			t.Fatalf("status = %q", resp.Status)
		}
	})

	t.Run("checkpoint/1", func(t *testing.T) {
		resp, err := client.Get("/checkpoint/1")
		if err != nil {
			t.Fatal(err)
		}
		if resp.Status != "ok" {
			t.Fatalf("status = %q", resp.Status)
		}
	})

	t.Run("api/health", func(t *testing.T) {
		resp, err := client.Get("/api/health")
		if err != nil {
			t.Fatal(err)
		}
		if resp.Status != "ok" || string(resp.Body) != "ok" {
			t.Fatalf("status=%q body=%q", resp.Status, resp.Body)
		}
	})

	t.Run("unknown", func(t *testing.T) {
		resp, err := client.Get("/unknown")
		if err != nil {
			t.Fatal(err)
		}
		if resp.Status != "not_found" {
			t.Fatalf("status = %q", resp.Status)
		}
	})

	t.Run("logother -> not_found", func(t *testing.T) {
		resp, err := client.Get("/logother")
		if err != nil {
			t.Fatal(err)
		}
		if resp.Status != "not_found" {
			t.Fatalf("status = %q", resp.Status)
		}
	})

	t.Run("checkpointother -> not_found", func(t *testing.T) {
		resp, err := client.Get("/checkpointother")
		if err != nil {
			t.Fatal(err)
		}
		if resp.Status != "not_found" {
			t.Fatalf("status = %q", resp.Status)
		}
	})
}

func TestVelocityLogServerWriteEntry(t *testing.T) {
	logStorage := &memLogStorage{}
	ml, err := nwep.NewMerkleLog(logStorage)
	if err != nil {
		t.Fatal(err)
	}
	defer ml.Free()

	ls, err := nwep.NewLogServer(ml, &nwep.LogServerSettings{
		Authorize: func(nodeid nwep.NodeID, entry *nwep.MerkleEntry) error {
			return nil // allow all
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	srv, client := startTestServer(t, WithLogServer(ls))
	defer func() {
		client.Close()
		srv.Shutdown()
	}()

	t.Run("write entry", func(t *testing.T) {
		entry := makeTestEntry(t)
		encoded, err := nwep.MerkleEntryEncode(entry)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := client.Post("/log/entry", encoded)
		if err != nil {
			t.Fatal(err)
		}
		if resp.Status != "created" && resp.Status != "ok" {
			t.Fatalf("status = %q", resp.Status)
		}
	})

	t.Run("size after write", func(t *testing.T) {
		resp, err := client.Get("/log/size")
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]int
		json.Unmarshal(resp.Body, &body)
		if body["size"] != 1 {
			t.Fatalf("size = %d, want 1", body["size"])
		}
	})

	t.Run("read entry back", func(t *testing.T) {
		resp, err := client.Get("/log/entry/0")
		if err != nil {
			t.Fatal(err)
		}
		if resp.Status != "ok" {
			t.Fatalf("status = %q", resp.Status)
		}
		if len(resp.Body) == 0 {
			t.Fatal("empty body")
		}
	})
}

func TestVelocityShutdownFreesLogAndAnchorServers(t *testing.T) {
	logStorage := &memLogStorage{}
	ml, err := nwep.NewMerkleLog(logStorage)
	if err != nil {
		t.Fatal(err)
	}
	defer ml.Free()

	ls, err := nwep.NewLogServer(ml, nil)
	if err != nil {
		t.Fatal(err)
	}

	blsKP, _ := nwep.BLSKeypairGenerate()
	anchorSet, _ := nwep.NewAnchorSet(1)
	defer anchorSet.Free()
	anchorSet.Add(blsKP.Pubkey(), true)
	as, _ := nwep.NewAnchorServer(blsKP, anchorSet, nil)

	srv, client := startTestServer(t,
		WithLogServer(ls),
		WithAnchorServer(as),
	)

	if srv.LogServer() == nil {
		t.Fatal("LogServer should be set")
	}
	if srv.AnchorServer() == nil {
		t.Fatal("AnchorServer should be set")
	}

	client.Close()
	srv.Shutdown()

	if srv.LogServer() != nil {
		t.Fatal("LogServer should be nil after Shutdown")
	}
	if srv.AnchorServer() != nil {
		t.Fatal("AnchorServer should be nil after Shutdown")
	}
}
