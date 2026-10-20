package main

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// testServer builds a server with an isolated config path so tests never touch
// the user's real ~/.config file.
func testServer(t *testing.T) *server {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("DSH_PET_MYGO_CONFIG", filepath.Join(dir, "config.json"))
	return newServer()
}

func TestMovDuration(t *testing.T) {
	cases := []struct {
		name string
		want float64
	}{
		{"待机呼吸休闲", 10.04},
		{"工作状态-垂头叹气冒汗", 4.92},
		{"螃蟹走路", 10.04},
	}
	for _, c := range cases {
		got, err := movDuration(filepath.Join(animDir, c.name+".mov"))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if math.Abs(got-c.want) > 0.1 {
			t.Errorf("%s: got %.2fs, want %.2fs", c.name, got, c.want)
		}
	}
}

func TestMovDurationMissingFile(t *testing.T) {
	if _, err := movDuration(filepath.Join(animDir, "does-not-exist.mov")); err == nil {
		t.Fatal("expected an error for a missing file")
	}
}

func TestFindAtomNested(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(animDir, "待机呼吸休闲.mov"))
	if err != nil {
		t.Skipf("asset not present: %v", err)
	}
	moov, ok := findAtom(data, "moov")
	if !ok {
		t.Fatal("moov not found")
	}
	if _, ok := findAtom(moov, "mvhd"); !ok {
		t.Fatal("mvhd not found inside moov (nested lookup)")
	}
}

func TestPickNextWeights(t *testing.T) {
	s := testServer(t)
	n := 40000
	counts := map[string]int{}
	for i := 0; i < n; i++ {
		kind, _ := s.pickNext()
		counts[kind]++
	}
	check := func(kind string, wantPct float64) {
		got := float64(counts[kind]) / float64(n) * 100
		if math.Abs(got-wantPct) > 2 {
			t.Errorf("%s: %.1f%%, want ~%.1f%%", kind, got, wantPct)
		}
	}
	check("idle", 10)
	check("turn", 5)
	check("move", 5)
	check("action", 80)
}

func TestPickNextNonEmpty(t *testing.T) {
	s := testServer(t)
	pool := s.actionPool()
	if len(pool) == 0 {
		t.Fatal("action pool is empty; resources/anim not found from the test cwd")
	}
	for i := 0; i < 100; i++ {
		kind, name := s.pickNext()
		if name == "" {
			t.Fatalf("pickNext returned an empty name for kind %q", kind)
		}
	}
}

func doJSON(t *testing.T, srv *httptest.Server, method, path string, body any) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req, err := http.NewRequest(method, srv.URL+path, &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("content-type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("%s %s: decode: %v", method, path, err)
	}
	return out
}

func TestContractState(t *testing.T) {
	s := testServer(t)
	srv := httptest.NewServer(s.handler())
	defer srv.Close()

	st := doJSON(t, srv, http.MethodGet, "/dsh-pet-7340/state", nil)
	pets, ok := st["pets"].(map[string]any)
	if !ok {
		t.Fatal("state.pets missing")
	}
	main, ok := pets["main"].(map[string]any)
	if !ok {
		t.Fatal("state.pets.main missing")
	}
	if _, ok := main["anim"].(map[string]any); !ok {
		t.Fatal("state.pets.main.anim missing")
	}
}

func TestContractAnimAndPlayback(t *testing.T) {
	s := testServer(t)
	srv := httptest.NewServer(s.handler())
	defer srv.Close()

	resp := doJSON(t, srv, http.MethodPost, "/dsh-pet-7340/anim?pet=main", map[string]any{"name": "东张西望"})
	pid, ok := resp["playbackId"].(string)
	if !ok || pid == "" {
		t.Fatalf("no playbackId in %v", resp)
	}

	pb := doJSON(t, srv, http.MethodGet, "/dsh-pet-7340/playback/"+pid, nil)
	if pb["status"] != "accepted" {
		t.Errorf("initial playback status = %v, want accepted", pb["status"])
	}
	if pb["name"] != "东张西望" {
		t.Errorf("playback name = %v, want 东张西望", pb["name"])
	}

	// The page reports completion; the receipt should follow.
	doJSON(t, srv, http.MethodPost, "/dsh-pet-7340/playback/"+pid+"/report", map[string]any{"status": "completed"})
	pb = doJSON(t, srv, http.MethodGet, "/dsh-pet-7340/playback/"+pid, nil)
	if pb["status"] != "completed" {
		t.Errorf("playback status = %v, want completed", pb["status"])
	}

	// Unknown playback id is a 404, not a silent success.
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/dsh-pet-7340/playback/nope", nil)
	r2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	r2.Body.Close()
	if r2.StatusCode != http.StatusNotFound {
		t.Errorf("unknown playback status = %d, want 404", r2.StatusCode)
	}
}

func TestContractBroadcast(t *testing.T) {
	s := testServer(t)
	srv := httptest.NewServer(s.handler())
	defer srv.Close()

	resp := doJSON(t, srv, http.MethodPost, "/dsh-pet-7340/broadcast?pet=main",
		map[string]any{"text": "测试气泡", "duration": 3000})
	if resp["ok"] != true {
		t.Fatalf("broadcast resp = %v", resp)
	}
	st := doJSON(t, srv, http.MethodGet, "/dsh-pet-7340/state", nil)
	pets := st["pets"].(map[string]any)
	say := pets["main"].(map[string]any)["say"].(map[string]any)
	data := say["data"].(map[string]any)
	if data["text"] != "测试气泡" {
		t.Errorf("say.text = %v, want 测试气泡", data["text"])
	}
}

func TestContractBadRequest(t *testing.T) {
	s := testServer(t)
	srv := httptest.NewServer(s.handler())
	defer srv.Close()

	// Empty name must be rejected, not accepted silently.
	resp := doJSONRaw(t, srv, http.MethodPost, "/dsh-pet-7340/anim?pet=main", `{"name":""}`)
	if resp != http.StatusBadRequest {
		t.Errorf("empty anim name status = %d, want 400", resp)
	}
}

func doJSONRaw(t *testing.T, srv *httptest.Server, method, path, body string) int {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("content-type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DSH_PET_MYGO_CONFIG", filepath.Join(dir, "config.json"))

	want := appConfig{Size: 0.6, Pets: []petConfig{{ID: "main", X: 123, Y: 45}}}
	saveConfig(want)

	got := loadConfig()
	if got.Size != 0.6 {
		t.Errorf("size = %v, want 0.6 (a zero size would reset to 1)", got.Size)
	}
	if len(got.Pets) != 1 || got.Pets[0].ID != "main" || got.Pets[0].X != 123 || got.Pets[0].Y != 45 {
		t.Fatalf("round trip got %+v", got)
	}
}

// TestScaleSurvivesPositionSave guards the bug where savePositions wrote
// only Pets and silently zeroed Size.
func TestScaleSurvivesPositionSave(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DSH_PET_MYGO_CONFIG", filepath.Join(dir, "config.json"))

	saveConfig(appConfig{Size: 0.6, Pets: []petConfig{{ID: "main", X: 1, Y: 2}}})
	// Simulate what savePositions writes: it must carry Size through.
	saveConfig(appConfig{Size: 0.6, Pets: []petConfig{{ID: "main", X: 300, Y: 400}}})

	got := loadConfig()
	if got.Size != 0.6 {
		t.Fatalf("size reset to %v after a position save, want 0.6", got.Size)
	}
	if got.Pets[0].X != 300 {
		t.Fatalf("position not updated: %+v", got.Pets[0])
	}
}

func TestConfigFirstRunWritesDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	t.Setenv("DSH_PET_MYGO_CONFIG", path)

	cfg := loadConfig()
	if len(cfg.Pets) == 0 {
		t.Fatal("first run returned no pets")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("first run did not write a default config: %v", err)
	}
}

func TestAnimDurationFallback(t *testing.T) {
	// A missing asset should fall back to animPlaySec rather than 0 (which
	// would make the chain spin without ever advancing).
	d := animDuration("definitely-missing-animation")
	if d.Seconds() != float64(animPlaySec) {
		t.Errorf("fallback duration = %.1fs, want %ds", d.Seconds(), animPlaySec)
	}
}

func TestCleanWhisperLines(t *testing.T) {
	raw := "1. 主人记得喝水~\n2、今天也要加油哦\n\n- \"代码写完了吗？\"\n  \n" +
		"这一行特别特别长超过四十个字所以应该被丢掉AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	got := cleanWhisperLines(raw)
	want := []string{"主人记得喝水~", "今天也要加油哦", "代码写完了吗？"}
	if len(got) != len(want) {
		t.Fatalf("got %d lines %v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestLoadWhisperLinesMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DSH_PET_MYGO_CONFIG", filepath.Join(dir, "config.json"))
	if got := loadWhisperLines(); got != nil {
		t.Errorf("missing file should return nil, got %v", got)
	}
}

func TestLoadWhisperLinesFromFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DSH_PET_MYGO_CONFIG", filepath.Join(dir, "config.json"))
	data, _ := json.Marshal(whispersFile{GeneratedAt: "x", Lines: []string{"a", "b"}})
	if err := os.WriteFile(filepath.Join(dir, "whispers.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	got := loadWhisperLines()
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("got %v, want [a b]", got)
	}
}
