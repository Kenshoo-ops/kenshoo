package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

const s3TestBucket = "db-backups"

// Page 1 holds an old backup and a folder placeholder that is the newest entry by date.
// The real newest backup is on page 2, so the test fails if paging or the placeholder skip breaks.
const s3ListPage1 = `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <Name>db-backups</Name>
  <Prefix>my-app/</Prefix>
  <KeyCount>2</KeyCount>
  <MaxKeys>2</MaxKeys>
  <IsTruncated>true</IsTruncated>
  <NextContinuationToken>page2</NextContinuationToken>
  <Contents><Key>my-app/</Key><LastModified>2026-10-28T00:00:00.000Z</LastModified><Size>0</Size></Contents>
  <Contents><Key>my-app/old.dump</Key><LastModified>2026-10-25T00:00:00.000Z</LastModified><Size>10</Size></Contents>
</ListBucketResult>`

const s3ListPage2 = `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <Name>db-backups</Name>
  <Prefix>my-app/</Prefix>
  <KeyCount>1</KeyCount>
  <MaxKeys>2</MaxKeys>
  <IsTruncated>false</IsTruncated>
  <Contents><Key>my-app/new.dump</Key><LastModified>2026-10-27T03:00:00.000Z</LastModified><Size>20</Size></Contents>
</ListBucketResult>`

const s3ListEmpty = `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <Name>db-backups</Name>
  <Prefix>my-app/</Prefix>
  <KeyCount>0</KeyCount>
  <MaxKeys>1000</MaxKeys>
  <IsTruncated>false</IsTruncated>
</ListBucketResult>`

const s3NoSuchKey = `<?xml version="1.0" encoding="UTF-8"?>
<Error><Code>NoSuchKey</Code><Message>The specified key does not exist.</Message></Error>`

// useFakeAWSEnv gives the SDK throwaway credentials and stops it from reading the real
// ~/.aws files or asking the cloud metadata service.
func useFakeAWSEnv(t *testing.T) {
	t.Helper()
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	t.Setenv("AWS_SESSION_TOKEN", "")
	t.Setenv("AWS_CONFIG_FILE", os.DevNull)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", os.DevNull)
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
}

// fakeS3 serves just enough of the path-style S3 API for these tests: ListObjectsV2 pages
// keyed by continuation token ("" for the first page), and GetObject.
func fakeS3(t *testing.T, lists map[string]string, objects map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		if r.URL.Path == "/"+s3TestBucket && r.URL.Query().Get("list-type") == "2" {
			body, ok := lists[r.URL.Query().Get("continuation-token")]
			if !ok {
				http.Error(w, "unexpected continuation token", http.StatusBadRequest)
				return
			}
			_, _ = io.WriteString(w, body)
			return
		}
		key := strings.TrimPrefix(r.URL.Path, "/"+s3TestBucket+"/")
		if data, ok := objects[key]; ok {
			_, _ = io.WriteString(w, data)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, s3NoSuchKey)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newTestS3Store(t *testing.T, srv *httptest.Server) *S3Store {
	t.Helper()
	useFakeAWSEnv(t)
	st, err := NewS3Store(context.Background(), S3Options{
		Endpoint: srv.URL,
		Region:   "us-east-1",
		Bucket:   s3TestBucket,
	})
	if err != nil {
		t.Fatalf("NewS3Store: %v", err)
	}
	return st
}

func TestS3Store_LatestWalksAllPagesAndSkipsFolderMarkers(t *testing.T) {
	srv := fakeS3(t, map[string]string{"": s3ListPage1, "page2": s3ListPage2}, nil)
	st := newTestS3Store(t, srv)

	got, err := st.Latest(context.Background(), "my-app/")
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if got.Key != "my-app/new.dump" || got.Size != 20 {
		t.Errorf("got key %q size %d, want my-app/new.dump size 20", got.Key, got.Size)
	}
	want := time.Date(2026, 10, 27, 3, 0, 0, 0, time.UTC)
	if !got.LastModified.Equal(want) {
		t.Errorf("got LastModified %v, want %v", got.LastModified, want)
	}
}

func TestS3Store_LatestNoBackup(t *testing.T) {
	srv := fakeS3(t, map[string]string{"": s3ListEmpty}, nil)
	st := newTestS3Store(t, srv)

	_, err := st.Latest(context.Background(), "my-app/")
	if !errors.Is(err, ErrNoBackup) {
		t.Fatalf("got %v, want ErrNoBackup", err)
	}
}

func TestS3Store_Download(t *testing.T) {
	srv := fakeS3(t, nil, map[string]string{"my-app/new.dump": "hello-backup"})
	st := newTestS3Store(t, srv)

	var buf bytes.Buffer
	if err := st.Download(context.Background(), "my-app/new.dump", &buf); err != nil {
		t.Fatalf("Download: %v", err)
	}
	if buf.String() != "hello-backup" {
		t.Errorf("got %q, want %q", buf.String(), "hello-backup")
	}
}

func TestS3Store_DownloadMissingKey(t *testing.T) {
	srv := fakeS3(t, nil, nil)
	st := newTestS3Store(t, srv)

	var buf bytes.Buffer
	if err := st.Download(context.Background(), "my-app/nope.dump", &buf); err == nil {
		t.Fatal("expected an error for a missing key, got nil")
	}
}

func TestNewS3Store_RequiresBucketAndRegion(t *testing.T) {
	_, err := NewS3Store(context.Background(), S3Options{Endpoint: "https://example.invalid"})
	if err == nil {
		t.Fatal("expected an error when bucket and region are missing")
	}
	for _, want := range []string{"bucket", "region"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should mention %q", err, want)
		}
	}
}
