package media

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/aidapedia/hayat/internal/config"
)

// s3Store talks to any S3-compatible store (AWS S3, Cloudflare R2, MinIO, …)
// with the three calls Hayat needs, signed with AWS Signature Version 4.
// Small on purpose: images are a few hundred KB, so no multipart, no streaming.
type s3Store struct {
	endpoint  *url.URL
	bucket    string
	prefix    string
	region    string
	accessKey string
	secretKey string
	pathStyle bool
	client    *http.Client
	now       func() time.Time
}

func newS3(cfg config.Storage) *s3Store {
	u, _ := url.Parse(cfg.S3Endpoint) // checked by config
	return &s3Store{endpoint: u, bucket: cfg.S3Bucket, prefix: cfg.S3Prefix, region: cfg.S3Region,
		accessKey: cfg.S3AccessKey, secretKey: cfg.S3SecretKey, pathStyle: cfg.S3PathStyle,
		client: &http.Client{Timeout: 30 * time.Second}, now: time.Now}
}

func (*s3Store) Name() string { return "s3" }

// Check is a no-op: a bucket can't be probed without writing to it, and a
// wrong key or bucket shows on the first upload with S3's own message.
func (*s3Store) Check(context.Context) error { return nil }

func (s *s3Store) objectURL(key string) string {
	u := *s.endpoint
	if s.pathStyle {
		u.Path = strings.TrimRight(u.Path, "/") + "/" + s.bucket + "/" + s.prefix + key
	} else {
		u.Host = s.bucket + "." + u.Host
		u.Path = strings.TrimRight(u.Path, "/") + "/" + s.prefix + key
	}
	return u.String()
}

func (s *s3Store) do(ctx context.Context, method, key, contentType string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, s.objectURL(key), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	sum := sha256.Sum256(body)
	sign(req, hex.EncodeToString(sum[:]), s.accessKey, s.secretKey, s.region, s.now())
	return s.client.Do(req)
}

func s3Error(op string, resp *http.Response) error {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	return fmt.Errorf("s3 %s: %s: %s", op, resp.Status, strings.TrimSpace(string(b)))
}

func (s *s3Store) Put(ctx context.Context, key, contentType string, data []byte) error {
	resp, err := s.do(ctx, http.MethodPut, key, contentType, data)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return s3Error("put", resp)
	}
	return nil
}

func (s *s3Store) Get(ctx context.Context, key string) ([]byte, error) {
	resp, err := s.do(ctx, http.MethodGet, key, "", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrMissing
	}
	if resp.StatusCode/100 != 2 {
		return nil, s3Error("get", resp)
	}
	return io.ReadAll(io.LimitReader(resp.Body, MaxUploadBytes+1))
}

func (s *s3Store) Delete(ctx context.Context, key string) error {
	resp, err := s.do(ctx, http.MethodDelete, key, "", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 && resp.StatusCode != http.StatusNotFound {
		return s3Error("delete", resp)
	}
	return nil
}

// ---------- AWS Signature Version 4 ----------

func hmacSHA256(key []byte, data string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(data))
	return m.Sum(nil)
}

// uriEncode escapes like S3 expects: everything but unreserved characters,
// and "/" kept as a separator in paths.
func uriEncode(s string, path bool) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-', c == '_', c == '.', c == '~':
			b.WriteByte(c)
		case c == '/' && path:
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// sign adds x-amz-date, x-amz-content-sha256 and Authorization to req,
// signing every header it already carries plus host.
func sign(req *http.Request, payloadHash, accessKey, secretKey, region string, now time.Time) {
	now = now.UTC()
	amzDate, day := now.Format("20060102T150405Z"), now.Format("20060102")
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	headers := map[string]string{"host": req.URL.Host}
	for k, v := range req.Header {
		headers[strings.ToLower(k)] = strings.Join(strings.Fields(strings.Join(v, ",")), " ")
	}
	names := make([]string, 0, len(headers))
	for k := range headers {
		names = append(names, k)
	}
	sort.Strings(names)
	var canonHeaders strings.Builder
	for _, k := range names {
		canonHeaders.WriteString(k + ":" + headers[k] + "\n")
	}
	signed := strings.Join(names, ";")

	q := req.URL.Query()
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var query []string
	for _, k := range keys {
		for _, v := range q[k] {
			query = append(query, uriEncode(k, false)+"="+uriEncode(v, false))
		}
	}
	path := req.URL.Path
	if path == "" {
		path = "/"
	}
	canonical := strings.Join([]string{req.Method, uriEncode(path, true), strings.Join(query, "&"), canonHeaders.String(), signed, payloadHash}, "\n")
	sum := sha256.Sum256([]byte(canonical))

	scope := day + "/" + region + "/s3/aws4_request"
	toSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(sum[:])
	key := hmacSHA256(hmacSHA256(hmacSHA256(hmacSHA256([]byte("AWS4"+secretKey), day), region), "s3"), "aws4_request")
	sig := hex.EncodeToString(hmacSHA256(key, toSign))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+accessKey+"/"+scope+", SignedHeaders="+signed+", Signature="+sig)
}
