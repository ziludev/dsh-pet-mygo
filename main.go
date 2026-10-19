// dsh-pet-mygo: a desktop pet that runs without DeepSeek Harness or Electron.
// A transparent, always-on-top MyGo window plays HEVC-with-alpha .mov assets,
// driven through dsh-pet's own /dsh-pet-7340 HTTP contract, so the existing
// pi bridge extension (and any other consumer) keeps working unchanged.
package main

import (
	"encoding/json"
	"log"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/egoist/mygo"
)

const (
	listenAddr = "127.0.0.1:3080"
	idleAnim   = "待机呼吸休闲"
	windowW    = 640
	windowH    = 360
)

// workStatusAnims is the animation pool for the six work-status tiers, in the
// same order dsh-pet uses (and the same names as resources/anim/*.mov).
var workStatusAnims = []string{
	"工作状态-思考冒泡",
	"工作状态-忙碌点按",
	"工作状态-清点归档",
	"工作状态-原地踱步张望",
	"工作状态-雀跃庆祝",
	"工作状态-垂头叹气冒汗",
}

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
	mu     sync.Mutex
	pets   map[string]*petState
	window *mygo.Window
}

func newServer() *server {
	p := &petState{
		Anim: field{Counter: 0, Data: map[string]any{"name": idleAnim}},
		Say:  field{Counter: 0, Data: nil},
	}
	return &server{pets: map[string]*petState{"main": p}}
}

func (s *server) setWindow(w *mygo.Window) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.window = w
}

func (s *server) windowPtr() *mygo.Window {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.window
}

func (s *server) pet(id string) *petState {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.pets[id]
	if p == nil {
		p = &petState{
			Anim: field{Counter: 0, Data: map[string]any{"name": idleAnim}},
			Say:  field{Counter: 0, Data: nil},
		}
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

// config returns the animation pool under dsh-pet's config shape, so the pi
// bridge extension's loadWorkAnims() reads a real pool instead of its fallback.
func (s *server) config() map[string]any {
	return map[string]any{
		"main": map[string]any{
			"animations": map[string]any{
				"idle":   []string{idleAnim},
				"events": map[string]any{"workStatus": workStatusAnims},
			},
		},
	}
}

// move places the window at normalized coordinates (0..1) over the primary
// display's work area.
func (s *server) move(x, y float64) {
	win := s.windowPtr()
	if win == nil {
		return
	}
	wa := mygo.Screen.PrimaryDisplay().WorkArea
	nx := float64(wa.X) + x*float64(wa.Width-windowW)
	ny := float64(wa.Y) + y*float64(wa.Height-windowH)
	win.SetPosition(int(nx), int(ny))
}

// throw flings the window: initial velocity from angle (degrees, 0 = right,
// 90 = down) and power (multiplier), bouncing off work-area edges with friction
// until it rests.
func (s *server) throw(power, angleDeg float64) {
	win := s.windowPtr()
	if win == nil {
		return
	}
	wa := mygo.Screen.PrimaryDisplay().WorkArea
	rad := angleDeg * math.Pi / 180
	speed := power * 2200 // px/s baseline
	vx := speed * math.Cos(rad)
	vy := speed * math.Sin(rad)

	x, y := win.Position()
	fx, fy := float64(x), float64(y)
	minX := float64(wa.X)
	maxX := float64(wa.X + wa.Width - windowW)
	minY := float64(wa.Y)
	maxY := float64(wa.Y + wa.Height - windowH)

	const dt = 1.0 / 60.0
	step := func() bool {
		fx += vx * dt
		fy += vy * dt
		if fx < minX {
			fx = minX
			vx = -vx * 0.82
		} else if fx > maxX {
			fx = maxX
			vx = -vx * 0.82
		}
		if fy < minY {
			fy = minY
			vy = -vy * 0.82
		} else if fy > maxY {
			fy = maxY
			vy = -vy * 0.82
		}
		win.SetPosition(int(fx), int(fy))
		vx *= 0.986
		vy *= 0.986
		return math.Abs(vx) < 6 && math.Abs(vy) < 6
	}
	for {
		if step() {
			return
		}
		time.Sleep(16 * time.Millisecond)
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
	postJSON := func(w http.ResponseWriter, r *http.Request, v any) bool {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return false
		}
		if err := json.NewDecoder(r.Body).Decode(v); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return false
		}
		return true
	}
	petOf := func(r *http.Request) string {
		if p := r.URL.Query().Get("pet"); p != "" {
			return p
		}
		return "main"
	}

	mux.HandleFunc("/dsh-pet-7340/state", cors(func(w http.ResponseWriter, r *http.Request) {
		jsonReply(w, s.state())
	}))

	mux.HandleFunc("/dsh-pet-7340/config", cors(func(w http.ResponseWriter, r *http.Request) {
		jsonReply(w, s.config())
	}))

	mux.HandleFunc("/dsh-pet-7340/anim", cors(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name string `json:"name"`
		}
		if !postJSON(w, r, &body) {
			return
		}
		if body.Name == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		s.setAnim(petOf(r), body.Name)
		jsonReply(w, map[string]bool{"ok": true})
	}))

	mux.HandleFunc("/dsh-pet-7340/broadcast", cors(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Text     string `json:"text"`
			Duration int    `json:"duration"`
		}
		if !postJSON(w, r, &body) {
			return
		}
		if body.Text == "" {
			http.Error(w, "text required", http.StatusBadRequest)
			return
		}
		s.setSay(petOf(r), body.Text, body.Duration)
		jsonReply(w, map[string]bool{"ok": true})
	}))

	mux.HandleFunc("/dsh-pet-7340/position", cors(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			X    float64 `json:"x"`
			Y    float64 `json:"y"`
			Mode string  `json:"mode"`
		}
		if !postJSON(w, r, &body) {
			return
		}
		body.X = math.Max(0, math.Min(1, body.X))
		body.Y = math.Max(0, math.Min(1, body.Y))
		s.move(body.X, body.Y)
		jsonReply(w, map[string]any{"ok": true, "x": body.X, "y": body.Y})
	}))

	mux.HandleFunc("/dsh-pet-7340/throw", cors(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Angle float64 `json:"angle"`
			Power float64 `json:"power"`
		}
		if !postJSON(w, r, &body) {
			return
		}
		if body.Power <= 0 {
			body.Power = 1
		}
		go s.throw(body.Power, body.Angle)
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
			Width:         windowW,
			Height:        windowH,
			Frameless:     true,
			Transparent:   true,
			AlwaysOnTop:   true,
			SkipTaskbar:   true,
			DisableShadow: true,
		})
		s.setWindow(win)
		if err := win.Page().LoadFile("resources/index.html"); err != nil {
			log.Fatalf("load pet page: %v", err)
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
