// dsh-pet-mygo: a desktop pet that runs without DeepSeek Harness or Electron.
// A transparent, always-on-top MyGo window plays HEVC-with-alpha .mov assets,
// driven through dsh-pet's own /dsh-pet-7340 HTTP contract, so the existing
// pi bridge extension (and any other consumer) keeps working unchanged.
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/egoist/mygo"
)

const (
	animDir   = "resources/anim"
	idleAnim  = "待机呼吸休闲"
	listenAddr = "127.0.0.1:3080"
)

// field mirrors dsh-pet's state leaf: { counter, data }. The counter is a
// strictly-increasing millisecond timestamp; consumers compare it to detect
// change, exactly as they do against the real dsh-pet.
type field struct {
	Counter int64       `json:"counter"`
	Data    interface{} `json:"data"`
}

// petState is the per-pet slice of /state the bridge extension reads.
type petState struct {
	Say  field `json:"say"`
	Anim field `json:"anim"`
}

type fullState struct {
	Sections map[string]field    `json:"sections"`
	Pets     map[string]petState `json:"pets"`
}

type server struct {
	mu   sync.Mutex
	pets map[string]*petState
}

func newServer() *server {
	p := &petState{
		Anim: field{Counter: 0, Data: map[string]any{"name": idleAnim}},
		Say:  field{Counter: 0, Data: nil},
	}
	return &server{pets: map[string]*petState{"main": p}}
}

func (s *server) pet(id string) *petState {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.pets[id]
	if p == nil {
		p = &petState{Anim: field{Counter: 0, Data: map[string]any{"name": idleAnim}}, Say: field{Counter: 0, Data: nil}}
		s.pets[id] = p
	}
	return p
}

func (s *server) setAnim(id, name string) {
	p := s.pet(id)
	p.Anim = field{Counter: time.Now().UnixMilli(), Data: map[string]any{"name": name}}
	log.Printf("anim %s -> %s", id, name)
}

func (s *server) setSay(id, text string, duration int) {
	if duration <= 0 {
		duration = 8000
	}
	p := s.pet(id)
	p.Say = field{Counter: time.Now().UnixMilli(), Data: map[string]any{"text": text, "duration": duration}}
	log.Printf("say %s -> %q", id, text)
}

func (s *server) state() fullState {
	s.mu.Lock()
	defer s.mu.Unlock()
	pets := make(map[string]petState, len(s.pets))
	for id, p := range s.pets {
		pets[id] = petState{Say: p.Say, Anim: p.Anim}
	}
	return fullState{
		// Sections exist for shape-compatibility; this port has no balance or
		// work-status source, so they stay idle like dsh-pet's standalone mode.
		Sections: map[string]field{
			"balance":    {Counter: 0, Data: nil},
			"workStatus": {Counter: 0, Data: nil},
			"notify":     {Counter: 0, Data: nil},
		},
		Pets: pets,
	}
}

func (s *server) handler() http.Handler {
	mux := http.NewServeMux()

	// The page is loaded over file://, so its fetches are cross-origin.
	cors := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "content-type")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			h(w, r)
		}
	}
	jsonReply := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}

	mux.HandleFunc("/dsh-pet-7340/state", cors(func(w http.ResponseWriter, r *http.Request) {
		jsonReply(w, s.state())
	}))

	mux.HandleFunc("/dsh-pet-7340/anim", cors(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		pet := r.URL.Query().Get("pet")
		if pet == "" {
			pet = "main"
		}
		s.setAnim(pet, body.Name)
		jsonReply(w, map[string]bool{"ok": true})
	}))

	mux.HandleFunc("/dsh-pet-7340/broadcast", cors(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Text     string `json:"text"`
			Duration int    `json:"duration"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Text == "" {
			http.Error(w, "text required", http.StatusBadRequest)
			return
		}
		pet := r.URL.Query().Get("pet")
		if pet == "" {
			pet = "main"
		}
		s.setSay(pet, body.Text, body.Duration)
		jsonReply(w, map[string]bool{"ok": true})
	}))

	mux.HandleFunc("/health", cors(func(w http.ResponseWriter, r *http.Request) {
		jsonReply(w, map[string]string{"name": "dsh-pet-mygo", "status": "ok"})
	}))

	return mux
}

func main() {
	s := newServer()
	go func() {
		log.Printf("serving /dsh-pet-7340 on http://%s", listenAddr)
		if err := http.ListenAndServe(listenAddr, s.handler()); err != nil {
			log.Fatalf("http server: %v", err)
		}
	}()

	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:         "dsh-pet",
			Width:         640,
			Height:        360,
			Frameless:     true,
			Transparent:   true,
			AlwaysOnTop:   true,
			SkipTaskbar:   true,
			DisableShadow: true,
		})
		if err := win.Page().LoadFile("resources/index.html"); err != nil {
			log.Fatalf("load pet page: %v", err)
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
