package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"
)

const (
	s3UnsignedPayload = "UNSIGNED-PAYLOAD"
	s3PartSize        = 8 << 20
)

type s3Client struct {
	endpoint  string
	region    string
	bucket    string
	access    string
	secret    string
	pathStyle bool
	http      *http.Client
}

type s3Part struct {
	Number int
	ETag   string
}

func newS3Client(endpoint, region, bucket, access, secret string) (*s3Client, error) {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if endpoint == "" || bucket == "" {
		return nil, fmt.Errorf("S3 endpoint and bucket are required")
	}
	if access == "" || secret == "" {
		return nil, fmt.Errorf("S3 credentials are not loaded")
	}
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "https://" + endpoint
	}
	if region == "" {
		region = "us-east-1"
	}
	return &s3Client{
		endpoint:  endpoint,
		region:    region,
		bucket:    bucket,
		access:    access,
		secret:    secret,
		pathStyle: true,
		http:      &http.Client{Timeout: 10 * time.Minute},
	}, nil
}

func (c *s3Client) objectURL(key string, query string) (*url.URL, error) {
	u, err := url.Parse(c.endpoint)
	if err != nil {
		return nil, err
	}
	if c.pathStyle {
		u.Path = "/" + c.bucket + "/" + strings.TrimPrefix(key, "/")
	} else {
		u.Host = c.bucket + "." + u.Host
		u.Path = "/" + strings.TrimPrefix(key, "/")
	}
	u.RawQuery = query
	return u, nil
}

func (c *s3Client) do(method, key, query string, body []byte, extra map[string]string) (*http.Response, error) {
	u, err := c.objectURL(key, query)
	if err != nil {
		return nil, err
	}
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, u.String(), rdr)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.ContentLength = int64(len(body))
		req.Header.Set("Content-Length", fmt.Sprintf("%d", len(body)))
	}
	for k, v := range extra {
		req.Header.Set(k, v)
	}
	payloadHash := s3UnsignedPayload
	if body != nil && len(body) < 1<<20 {
		sum := sha256.Sum256(body)
		payloadHash = hex.EncodeToString(sum[:])
	}
	c.sign(req, payloadHash, time.Now().UTC())
	return c.http.Do(req)
}

func (c *s3Client) sign(req *http.Request, payloadHash string, now time.Time) {
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", payloadHash)
	host := req.URL.Host
	req.Header.Set("Host", host)

	headers := map[string]string{
		"host":                 host,
		"x-amz-content-sha256": payloadHash,
		"x-amz-date":           amzDate,
	}
	var names []string
	for n := range headers {
		names = append(names, n)
	}
	sort.Strings(names)
	var canon bytes.Buffer
	for _, n := range names {
		canon.WriteString(n)
		canon.WriteByte(':')
		canon.WriteString(strings.TrimSpace(headers[n]))
		canon.WriteByte('\n')
	}
	signed := strings.Join(names, ";")
	canonicalQuery := canonicalS3Query(req.URL.RawQuery)
	canonicalRequest := strings.Join([]string{
		req.Method,
		req.URL.EscapedPath(),
		canonicalQuery,
		canon.String(),
		signed,
		payloadHash,
	}, "\n")
	sum := sha256.Sum256([]byte(canonicalRequest))
	scope := dateStamp + "/" + c.region + "/s3/aws4_request"
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		hex.EncodeToString(sum[:]),
	}, "\n")
	sig := hex.EncodeToString(hmacSHA256(s3SigningKey(c.secret, dateStamp, c.region), stringToSign))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+c.access+"/"+scope+", SignedHeaders="+signed+", Signature="+sig)
}

func canonicalS3Query(raw string) string {
	if raw == "" {
		return ""
	}
	vals, err := url.ParseQuery(raw)
	if err != nil {
		return raw
	}
	var keys []string
	for k := range vals {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		vs := vals[k]
		if len(vs) == 0 {
			parts = append(parts, url.QueryEscape(k)+"=")
			continue
		}
		sort.Strings(vs)
		for _, v := range vs {
			parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(v))
		}
	}
	return strings.Join(parts, "&")
}

func s3SigningKey(secret, date, region string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), date)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, "s3")
	return hmacSHA256(kService, "aws4_request")
}

func hmacSHA256(key []byte, data string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(data))
	return m.Sum(nil)
}

func (c *s3Client) pingBucket() error {
	short := *c
	short.http = &http.Client{Timeout: 45 * time.Second}
	resp, err := short.do(http.MethodGet, "", "list-type=2&max-keys=1", nil, nil)
	if err != nil {
		return fmt.Errorf("cannot reach %s: %v", c.endpoint, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode == 200 || resp.StatusCode == 204 {
		return nil
	}
	if resp.StatusCode == 404 {
		return fmt.Errorf("bucket %q was not found at %s", c.bucket, c.endpoint)
	}
	if resp.StatusCode == 403 {
		return fmt.Errorf("access denied listing bucket %q (keys or permissions)", c.bucket)
	}
	return fmt.Errorf("S3 %s: %s", resp.Status, truncateS3Err(body))
}

func pingSavedS3() error {
	backupMu.RLock()
	d := activeDestinationLocked()
	backupMu.RUnlock()
	client, err := newS3Client(d.Endpoint, d.Region, d.Bucket, destAccessKey(d.ID), destSecretKey(d.ID))
	if err != nil {
		return err
	}
	return client.pingBucket()
}

func (c *s3Client) createMultipart(key string) (string, error) {
	resp, err := c.do(http.MethodPost, key, "uploads=", nil, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("S3 CreateMultipartUpload %s: %s", resp.Status, truncateS3Err(body))
	}
	var parsed struct {
		UploadID string `xml:"UploadId"`
	}
	if err := xml.Unmarshal(body, &parsed); err != nil || parsed.UploadID == "" {
		return "", fmt.Errorf("S3 CreateMultipartUpload: missing upload id")
	}
	return parsed.UploadID, nil
}

func (c *s3Client) uploadPart(key, uploadID string, number int, data []byte) (string, error) {
	q := fmt.Sprintf("partNumber=%d&uploadId=%s", number, url.QueryEscape(uploadID))
	resp, err := c.do(http.MethodPut, key, q, data, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("S3 UploadPart %d %s: %s", number, resp.Status, truncateS3Err(body))
	}
	etag := strings.Trim(resp.Header.Get("ETag"), `"`)
	if etag == "" {
		return "", fmt.Errorf("S3 UploadPart %d: missing ETag", number)
	}
	return etag, nil
}

func (c *s3Client) completeMultipart(key, uploadID string, parts []s3Part) error {
	var buf bytes.Buffer
	buf.WriteString(`<CompleteMultipartUpload>`)
	for _, p := range parts {
		etag := p.ETag
		if !strings.HasPrefix(etag, `"`) {
			etag = `"` + etag + `"`
		}
		fmt.Fprintf(&buf, `<Part><PartNumber>%d</PartNumber><ETag>%s</ETag></Part>`, p.Number, xmlEscape(etag))
	}
	buf.WriteString(`</CompleteMultipartUpload>`)
	q := "uploadId=" + url.QueryEscape(uploadID)
	resp, err := c.do(http.MethodPost, key, q, buf.Bytes(), map[string]string{
		"Content-Type": "application/xml",
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("S3 CompleteMultipartUpload %s: %s", resp.Status, truncateS3Err(body))
	}
	return nil
}

func (c *s3Client) abortMultipart(key, uploadID string) {
	q := "uploadId=" + url.QueryEscape(uploadID)
	resp, err := c.do(http.MethodDelete, key, q, nil, nil)
	if err != nil {
		log.Printf("S3 abort multipart failed: %v", err)
		return
	}
	resp.Body.Close()
}

func (c *s3Client) putObject(key string, data []byte, contentType string) error {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	resp, err := c.do(http.MethodPut, key, "", data, map[string]string{
		"Content-Type": contentType,
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("S3 PutObject %s: %s", resp.Status, truncateS3Err(body))
	}
	return nil
}

func (c *s3Client) streamUpload(key string, r io.Reader, onBytes func(uint64)) (uint64, error) {
	uploadID, err := c.createMultipart(key)
	if err != nil {
		return 0, err
	}
	var parts []s3Part
	buf := make([]byte, s3PartSize)
	npart := 0
	var total uint64
	for {
		nr, rerr := io.ReadFull(r, buf)
		if nr > 0 {
			npart++
			chunk := buf[:nr]
			etag, uerr := c.uploadPart(key, uploadID, npart, chunk)
			if uerr != nil {
				c.abortMultipart(key, uploadID)
				return total, uerr
			}
			parts = append(parts, s3Part{Number: npart, ETag: etag})
			total += uint64(nr)
			if onBytes != nil {
				onBytes(total)
			}
		}
		if rerr == io.EOF || rerr == io.ErrUnexpectedEOF {
			break
		}
		if rerr != nil {
			c.abortMultipart(key, uploadID)
			return total, rerr
		}
	}
	if npart == 0 {
		npart = 1
		etag, uerr := c.uploadPart(key, uploadID, 1, []byte{})
		if uerr != nil {
			c.abortMultipart(key, uploadID)
			return 0, uerr
		}
		parts = append(parts, s3Part{Number: 1, ETag: etag})
	}
	if err := c.completeMultipart(key, uploadID, parts); err != nil {
		c.abortMultipart(key, uploadID)
		return total, err
	}
	return total, nil
}

func xmlEscape(s string) string {
	var b strings.Builder
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

func truncateS3Err(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 400 {
		return s[:400]
	}
	return s
}

func s3ObjectURI(endpoint, bucket, key string) string {
	endpoint = strings.TrimRight(endpoint, "/")
	return endpoint + "/" + bucket + "/" + strings.TrimPrefix(key, "/")
}

func joinS3Key(prefix, name string) string {
	prefix = strings.Trim(prefix, "/")
	name = strings.TrimPrefix(name, "/")
	if prefix == "" {
		return name
	}
	return path.Join(prefix, name)
}
