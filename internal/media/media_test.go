package media

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aidapedia/hayat/internal/config"
	"github.com/aidapedia/hayat/internal/store"
	"github.com/aidapedia/hayat/internal/testdb"
)

// TestMain stops the package's embedded PostgreSQL when its tests finish.
func TestMain(m *testing.M) { os.Exit(testdb.Main(m)) }

func picture(t *testing.T, w, h int, alpha bool) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			a := uint8(255)
			if alpha && x < w/2 {
				a = 0
			}
			img.Set(x, y, color.NRGBA{uint8(x), uint8(y), 120, a})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestPrepareResizesAndReencodes(t *testing.T) {
	p, err := prepare(Avatar, picture(t, 800, 400, false))
	if err != nil || p.contentType != "image/jpeg" || p.width != 256 || p.height != 256 {
		t.Fatalf("a photo avatar is a 256 square JPEG, got %s %dx%d, %v", p.contentType, p.width, p.height, err)
	}
	if _, err := jpeg.Decode(bytes.NewReader(p.data)); err != nil {
		t.Fatalf("the kept avatar must decode: %v", err)
	}
	if p, _ := prepare(Avatar, picture(t, 64, 64, true)); p.contentType != "image/png" || p.width != 64 {
		t.Fatalf("a transparent avatar stays PNG and isn't enlarged, got %s %d", p.contentType, p.width)
	}
	if p, _ := prepare(Logo, picture(t, 1000, 500, true)); p.width != 512 || p.height != 256 || p.contentType != "image/png" {
		t.Fatalf("a logo fits 512 and keeps its shape, got %dx%d %s", p.width, p.height, p.contentType)
	}
	for name, data := range map[string][]byte{
		"svg":   []byte(`<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"/>`),
		"html":  []byte("<html><script>alert(1)</script></html>"),
		"empty": nil,
		"huge":  make([]byte, MaxUploadBytes+1),
	} {
		if _, err := prepare(Logo, data); !errors.Is(err, ErrBadImage) {
			t.Errorf("%s must be refused, got %v", name, err)
		}
	}
}

// The GET Object example from AWS's Signature Version 4 documentation.
func TestSignMatchesAWSExample(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://examplebucket.s3.amazonaws.com/test.txt", nil)
	req.Header.Set("Range", "bytes=0-9")
	sign(req, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		"AKIAIOSFODNN7EXAMPLE", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "us-east-1",
		time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC))
	want := "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, " +
		"SignedHeaders=host;range;x-amz-content-sha256;x-amz-date, Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"
	if got := req.Header.Get("Authorization"); got != want {
		t.Fatalf("signature:\n got %s\nwant %s", got, want)
	}
}

// fakeS3 keeps objects in memory and checks each request is signed.
type fakeS3 struct {
	mu      sync.Mutex
	objects map[string][]byte
	paths   []string
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=AK/") || r.Header.Get("X-Amz-Content-Sha256") == "" {
		http.Error(w, "unsigned", http.StatusForbidden)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.paths = append(f.paths, r.Method+" "+r.URL.Path)
	switch r.Method {
	case http.MethodPut:
		b, _ := io.ReadAll(r.Body)
		f.objects[r.URL.Path] = b
	case http.MethodGet:
		b, ok := f.objects[r.URL.Path]
		if !ok {
			http.Error(w, "<Error><Code>NoSuchKey</Code></Error>", http.StatusNotFound)
			return
		}
		_, _ = w.Write(b)
	case http.MethodDelete:
		delete(f.objects, r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	}
}

func TestS3Store(t *testing.T) {
	fake := &fakeS3{objects: map[string][]byte{}}
	srv := httptest.NewServer(fake)
	defer srv.Close()
	s, err := NewStore(config.Storage{Backend: "s3", S3Endpoint: srv.URL, S3Region: "auto", S3Bucket: "media", S3Prefix: "hayat/",
		S3AccessKey: "AK", S3SecretKey: "SK", S3PathStyle: true})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	key := "0193a3c0-0000-7000-8000-000000000001.png"
	if err := s.Put(ctx, key, "image/png", []byte("img")); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Get(ctx, key); err != nil || string(got) != "img" {
		t.Fatalf("get: %q %v", got, err)
	}
	if err := s.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, key); !errors.Is(err, ErrMissing) {
		t.Fatalf("a deleted object is missing, got %v", err)
	}
	if fake.paths[0] != "PUT /media/hayat/"+key {
		t.Fatalf("path-style keys go under the bucket and prefix, got %v", fake.paths)
	}

	vhost := newS3(config.Storage{S3Endpoint: "https://s3.amazonaws.com", S3Bucket: "media", S3Prefix: "p/"})
	if u := vhost.objectURL(key); u != "https://media.s3.amazonaws.com/p/"+key {
		t.Fatalf("virtual-hosted URL: %s", u)
	}
}

func TestLocalStore(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(config.Storage{Backend: "local", Dir: dir + "/media"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.Check(ctx); err != nil {
		t.Fatalf("check makes the directory and writes to it: %v", err)
	}
	if _, err := NewStore(config.Storage{Backend: "db"}); err == nil {
		t.Fatal("images are never kept in the database")
	}
	if ro := (localStore{dir: "/proc/hayat-cannot-write-here"}); ro.Check(ctx) == nil {
		t.Fatal("a directory that can't be written must fail the check")
	}
	key := "0193a3c0-0000-7000-8000-000000000002.jpg"
	if err := s.Put(ctx, key, "image/jpeg", []byte("jpg")); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Get(ctx, key); err != nil || string(got) != "jpg" {
		t.Fatalf("get: %q %v", got, err)
	}
	if err := s.Put(ctx, "../../etc/passwd", "text/plain", []byte("x")); err == nil {
		t.Fatal("a key that isn't ours must be refused")
	}
	_ = s.Delete(ctx, key)
	if _, err := s.Get(ctx, key); !errors.Is(err, ErrMissing) {
		t.Fatalf("deleted: %v", err)
	}
}

func TestServiceKeepsServesAndForgets(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	for _, backend := range []string{"local", "s3"} {
		fake := httptest.NewServer(&fakeS3{objects: map[string][]byte{}})
		defer fake.Close()
		blobs, err := NewStore(config.Storage{Backend: backend, Dir: t.TempDir(), S3Endpoint: fake.URL, S3Bucket: "b",
			S3AccessKey: "AK", S3SecretKey: "SK", S3PathStyle: true})
		if err != nil {
			t.Fatal(err)
		}
		m := New(db, blobs, "https://auth.example.com/", slog.New(slog.NewTextHandler(io.Discard, nil)))

		md, err := m.Upload(ctx, Avatar, Owner("user", "u1"), picture(t, 300, 300, false), "test")
		if err != nil {
			t.Fatalf("%s: %v", backend, err)
		}
		url := m.URL(md.ID)
		if id, ok := m.IDFromURL(url); !ok || id != md.ID || url != "https://auth.example.com/media/"+md.ID.String() {
			t.Fatalf("%s: url %s", backend, url)
		}
		if _, ok := m.IDFromURL("https://lh3.googleusercontent.com/a/xyz"); ok {
			t.Fatalf("%s: someone else's URL isn't ours", backend)
		}
		kind, data, err := m.Open(ctx, md.ID)
		if err != nil || kind != "image/jpeg" || len(data) != md.Bytes {
			t.Fatalf("%s: open %s %d, %v", backend, kind, len(data), err)
		}
		if owner, _ := m.OwnerOf(ctx, md.ID); owner != "user:u1" {
			t.Fatalf("%s: owner %q", backend, owner)
		}

		next, err := m.Replace(ctx, Avatar, Owner("user", "u1"), picture(t, 40, 40, false), md.ID, "test")
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := m.Open(ctx, md.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("%s: a replaced image is gone, got %v", backend, err)
		}
		if _, _, err := m.Open(ctx, next.ID); err != nil {
			t.Fatalf("%s: the new image opens: %v", backend, err)
		}
	}
}

func TestAdoptLoginPageAssets(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	blobs, _ := NewStore(config.Storage{Backend: "local", Dir: t.TempDir()})
	m := New(db, blobs, "https://auth.example.com", nil)
	id := store.NewID()
	logo := picture(t, 8, 8, true)
	if _, err := db.Pool.Exec(ctx, `INSERT INTO login_page_assets (id, service_id, content_type, data, created_by) VALUES ($1, 'billing', 'image/png', $2, 'test')`, id, logo); err != nil {
		t.Fatal(err)
	}
	for round, want := range []int{1, 0} {
		n, err := m.AdoptLoginPageAssets(ctx)
		if err != nil || n != want {
			t.Fatalf("round %d: moved %d, want %d, %v", round, n, want, err)
		}
	}
	kind, data, err := m.Open(ctx, id)
	if err != nil || kind != "image/png" || !bytes.Equal(data, logo) {
		t.Fatalf("an adopted logo keeps its id and bytes: %s %d, %v", kind, len(data), err)
	}
	if owner, _ := m.OwnerOf(ctx, id); owner != "login_page:billing" {
		t.Fatalf("owner %q", owner)
	}
}
