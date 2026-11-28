package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// A checksum file written next to the dump is newer than the dump. Without a suffix filter
// it would be picked as "the latest backup".
var suffixCases = []struct {
	name, suffix, want string
	noBackup           bool
}{
	{name: "suffix skips the newer checksum file and other case", suffix: ".dump", want: "my-app/a.dump"},
	{name: "suffix matches nothing", suffix: ".nomatch", noBackup: true},
	{name: "empty suffix keeps the newest object", suffix: "", want: "my-app/B.DUMP"},
}

func TestMemStoreLatestSuffix(t *testing.T) {
	s := NewMemStore()
	base := time.Date(2026, 10, 26, 3, 0, 0, 0, time.UTC)
	s.Put("my-app/a.dump", []byte("d"), base)
	s.Put("my-app/a.dump.sha256", []byte("c"), base.Add(24*time.Hour))
	s.Put("my-app/B.DUMP", []byte("u"), base.Add(48*time.Hour))
	for _, tc := range suffixCases {
		got, err := s.Latest(context.Background(), "my-app/", tc.suffix)
		checkSuffixResult(t, tc.name, tc.want, tc.noBackup, got, err)
	}
}

func TestS3StoreLatestSuffix(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Name>` + s3TestBucket + `</Name><Prefix>my-app/</Prefix><KeyCount>3</KeyCount><MaxKeys>1000</MaxKeys><IsTruncated>false</IsTruncated>`)
	for _, o := range [][2]string{
		{"my-app/a.dump", "2026-10-26T03:00:00.000Z"},
		{"my-app/a.dump.sha256", "2026-10-27T03:00:00.000Z"},
		{"my-app/B.DUMP", "2026-10-28T03:00:00.000Z"},
	} {
		fmt.Fprintf(&b, `<Contents><Key>%s</Key><LastModified>%s</LastModified><ETag>"x"</ETag><Size>10</Size><StorageClass>STANDARD</StorageClass></Contents>`, o[0], o[1])
	}
	b.WriteString(`</ListBucketResult>`)

	st := newTestS3Store(t, fakeS3(t, map[string]string{"": b.String()}, nil))
	for _, tc := range suffixCases {
		got, err := st.Latest(context.Background(), "my-app/", tc.suffix)
		checkSuffixResult(t, tc.name, tc.want, tc.noBackup, got, err)
	}
}

func checkSuffixResult(t *testing.T, name, want string, noBackup bool, got Object, err error) {
	t.Helper()
	if noBackup {
		if !errors.Is(err, ErrNoBackup) {
			t.Errorf("%s: got %q, %v; want ErrNoBackup", name, got.Key, err)
		}
		return
	}
	if err != nil {
		t.Errorf("%s: unexpected error: %v", name, err)
		return
	}
	if got.Key != want {
		t.Errorf("%s: got %q, want %q", name, got.Key, want)
	}
}
