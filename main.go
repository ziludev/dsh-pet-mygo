// dsh-pet-mygo: a desktop pet that runs without DeepSeek Harness or Electron.
// A transparent, always-on-top MyGo window per pet plays HEVC-with-alpha .mov
// assets, driven through dsh-pet's own /dsh-pet-7340 HTTP contract, so the
// existing pi bridge extension (and any other consumer) keeps working unchanged.
package main

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"math/rand"
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

// roamAnims are the walk animations played while the pet roams across screen.
var roamAnims = []string{"螃蟹走路", "原地漂浮踏步", "原地左转奔跑"}

// petConfig lists the pets to spawn; each gets its own window.
type petConfig struct {
	ID string
	X  int // initial top-left, logical coords
	Y  int
}

var defaultPets = []petConfig{
	{ID: "main", X: 700, Y: 220},
	{ID: "side", X: 980, Y: 420},
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
	mu      sync.Mutex
	pets    map[string]*petState
	windows map[string]*mygo.Window   // pet id -> window
	winPet  map[*mygo.Window]string   // window -> pet id
	dragGen map[*mygo.Window]int64    // bumped on manual move, cancels an in-flight fling
}

func newServer() *server {
	p := &petState{
		Anim: field{Counter: 0, Data: map[string]any{"name": idleAnim}},
		Say:  field{Counter: 0, Data: nil},
	}
	return &server{
		pets:    map[string]*petState{"main": p},
		windows: map[string]*mygo.Window{},
		winPet:  map[*mygo.Window]string{},
		dragGen: map[*mygo.Window]int64{},
	}
}

// addWindow registers a window for a pet and seeds its state.
func (s *server) addWindow(id string, w *mygo.Window) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.windows[id] = w
	s.winPet[w] = id
	if s.pets[id] == nil {
		s.pets[id] = &petState{
			Anim: field{Counter: 0, Data: map[string]any{"name": idleAnim}},
			Say:  field{Counter: 0, Data: nil},
		}
	}
}

func (s *server) windowFor(id string) *mygo.Window {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.windows[id]
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

// beginMove bumps the window's drag generation so any active fling or roam
// stops, and returns the new generation for the move that's about to run.
func (s *server) beginMove(w *mygo.Window) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dragGen[w]++
	return s.dragGen[w]
}

// move places a pet's window at normalized coordinates (0..1) over the primary
// display's work area.
func (s *server) move(id string, x, y float64) {
	win := s.windowFor(id)
	if win == nil {
		return
	}
	s.beginMove(win)
	wa := mygo.Screen.PrimaryDisplay().WorkArea
	nx := float64(wa.X) + x*float64(wa.Width-windowW)
	ny := float64(wa.Y) + y*float64(wa.Height-windowH)
	win.SetPosition(int(nx), int(ny))
}

// fling moves a window under an initial velocity (px/s), bouncing off work-area
// edges with friction until it rests or a manual move cancels it.
func (s *server) fling(win *mygo.Window, vx, vy float64) {
	wa := mygo.Screen.PrimaryDisplay().WorkArea
	gen := s.beginMove(win)

	x, y := win.Position()
	fx, fy := float64(x), float64(y)
	minX := float64(wa.X)
	maxX := float64(wa.X + wa.Width - windowW)
	minY := float64(wa.Y)
	maxY := float64(wa.Y + wa.Height - windowH)

	const dt = 1.0 / 60.0
	step := func() bool {
		s.mu.Lock()
		cancelled := s.dragGen[win] != gen
		s.mu.Unlock()
		if cancelled {
			return true
		}
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

// throw flings a pet by angle (degrees, 0 = right, 90 = down) and power.
func (s *server) throw(id string, power, angleDeg float64) {
	win := s.windowFor(id)
	if win == nil {
		return
	}
	rad := angleDeg * math.Pi / 180
	speed := power * 2200
	s.fling(win, speed*math.Cos(rad), speed*math.Sin(rad))
}

// roam walks a pet a short distance in a random direction, probing the work-area
// edges so it never walks off screen. The walk animation plays while the window
// slides, then the pet returns to idle.
func (s *server) roam(id string) {
	win := s.windowFor(id)
	if win == nil {
		return
	}
	wa := mygo.Screen.PrimaryDisplay().WorkArea
	x, y := win.Position()

	leftSpace := x - wa.X
	rightSpace := (wa.X + wa.Width - windowW) - x
	const minSpace = 200
	var dirs []int
	if leftSpace > minSpace {
		dirs = append(dirs, -1)
	}
	if rightSpace > minSpace {
		dirs = append(dirs, 1)
	}
	if len(dirs) == 0 {
		return
	}
	dir := dirs[rand.Intn(len(dirs))]

	room := leftSpace
	if dir > 0 {
		room = rightSpace
	}
	maxDist := room - 20
	if maxDist < 60 {
		maxDist = 60
	}
	dist := 60 + rand.Intn(maxDist-60+1)

	s.setAnim(id, roamAnims[rand.Intn(len(roamAnims))])
	gen := s.beginMove(win)

	const moveDur = 2000 * time.Millisecond
	const steps = 24
	stepDur := moveDur / steps
	for i := 1; i <= steps; i++ {
		s.mu.Lock()
		cancelled := s.dragGen[win] != gen
		s.mu.Unlock()
		if cancelled {
			return
		}
		win.SetPosition(x+dir*dist*i/steps, y)
		time.Sleep(stepDur)
	}
	s.setAnim(id, idleAnim)
}

// autoRoam periodically walks a pet while it is idle, until the process ends.
func (s *server) autoRoam(id string) {
	for {
		time.Sleep(time.Duration(10+rand.Intn(10)) * time.Second)
		s.roam(id)
	}
}

// Pet is the service bound to each page via mygo.Bind. Bridge methods take the
// calling window from the context, so each pet's page moves its own window.
type Pet struct{ server *server }

// Ping lets a page verify the bridge is wired up.
func (p *Pet) Ping() string { return "pong" }

// WhoAmI returns the pet id of the window that called it.
func (p *Pet) WhoAmI(ctx context.Context) string {
	w := mygo.CallerWindow(ctx)
	p.server.mu.Lock()
	defer p.server.mu.Unlock()
	if id, ok := p.server.winPet[w]; ok {
		return id
	}
	return "main"
}

// MoveTo moves the calling window's top-left corner to a screen position.
func (p *Pet) MoveTo(ctx context.Context, x, y int) {
	w := mygo.CallerWindow(ctx)
	p.server.beginMove(w)
	w.SetPosition(x, y)
}

// Fling throws the calling window with an initial velocity (px/s), from a drag
// release.
func (p *Pet) Fling(ctx context.Context, vx, vy float64) {
	log.Printf("fling vx=%.0f vy=%.0f", vx, vy)
	p.server.fling(mygo.CallerWindow(ctx), vx, vy)
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
		s.move(petOf(r), body.X, body.Y)
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
		go s.throw(petOf(r), body.Power, body.Angle)
		jsonReply(w, map[string]bool{"ok": true})
	}))

	mux.HandleFunc("/health", cors(func(w http.ResponseWriter, r *http.Request) {
		jsonReply(w, map[string]string{"name": "dsh-pet-mygo", "status": "ok"})
	}))

	return mux
}

func main() {
	s := newServer()
	mygo.Bind(&Pet{server: s})
	go func() {
		log.Printf("serving /dsh-pet-7340 on http://%s", listenAddr)
		if err := http.ListenAndServe(listenAddr, s.handler()); err != nil {
			log.Fatalf("http server: %v", err)
		}
	}()

	mygo.App.WhenReady(func() {
		for _, pc := range defaultPets {
			pc := pc
			win := mygo.NewWindow(mygo.WindowOptions{
				Title:         "dsh-pet",
				Width:         windowW,
				Height:        windowH,
				X:             pc.X,
				Y:             pc.Y,
				Frameless:     true,
				Transparent:   true,
				AlwaysOnTop:   true,
				SkipTaskbar:   true,
				DisableShadow: true,
			})
			s.addWindow(pc.ID, win)
			if err := win.Page().LoadFile("resources/index.html"); err != nil {
				log.Fatalf("load pet page: %v", err)
			}
			go s.autoRoam(pc.ID)
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
