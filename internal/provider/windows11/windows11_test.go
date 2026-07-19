package windows11

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/net/dns/dnsmessage"

	"github.com/arnaudcharles/iso-auto-downloader/internal/provider"
)

// withTestServer stands up a mock of Microsoft's 4-step consumer flow.
// sentinelReject makes step 4 return the anti-bot rejection instead of a
// download link, so the datacenter-IP failure path can be tested too.
func withTestServer(t *testing.T, sentinelReject bool) {
	t.Helper()

	const isoContent = "fake windows 11 iso bytes"

	mux := http.NewServeMux()
	mux.HandleFunc("/software-download/windows11", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<select><option value="3321">Windows 11 (multi-edition ISO for x64 devices)</option></select>`))
	})
	mux.HandleFunc("/software-download/windows11arm64", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<select><option value="3324">Windows 11 (multi-edition ISO for Arm64)</option></select>`))
	})
	mux.HandleFunc("/tags", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/mdt.js", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`var x = "?w=deadbeef1234"; rticks="+56789";`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/skuinfo", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"Skus":[
			{"Id":"20035","Language":"Arabic","ProductDisplayName":"Windows 11 25H2__V2"},
			{"Id":"20047","Language":"English International","ProductDisplayName":"Windows 11 25H2__V2"}
		]}`))
	})
	mux.HandleFunc("/downloadlinks", func(w http.ResponseWriter, r *http.Request) {
		if sentinelReject {
			_, _ = w.Write([]byte(`{"Errors":[{"Value":"Sentinel marked this request as rejected."}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"ProductDownloadOptions":[
			{"Uri":"http://` + r.Host + `/iso/Win11_25H2_EnglishInternational_x64.iso"},
			{"Uri":"http://` + r.Host + `/iso/Win11_25H2_EnglishInternational_Arm64_v2.iso"}
		]}`))
	})
	mux.HandleFunc("/iso/Win11_25H2_EnglishInternational_x64.iso", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(isoContent))
	})
	// Real report: Microsoft's actual response capitalizes the architecture
	// ("Arm64"), while this provider's own Variant.Arch is lowercase
	// ("arm64") — this mock deliberately matches Microsoft's real casing so
	// this test would have caught the case-sensitive match bug.
	mux.HandleFunc("/iso/Win11_25H2_EnglishInternational_Arm64_v2.iso", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(isoContent))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	origPages, origAuth, origMdt, origReply, origSku, origLink := downloadPageURLs, sessionAuthURL, ovDFMdtURL, ovDFReplyURL, skuInfoURL, downloadLinkURL
	downloadPageURLs = map[string]string{
		"x64":   srv.URL + "/software-download/windows11",
		"arm64": srv.URL + "/software-download/windows11arm64",
	}
	sessionAuthURL = srv.URL + "/tags"
	ovDFMdtURL = srv.URL + "/mdt.js"
	ovDFReplyURL = srv.URL + "/"
	skuInfoURL = srv.URL + "/skuinfo"
	downloadLinkURL = srv.URL + "/downloadlinks"
	t.Cleanup(func() {
		downloadPageURLs, sessionAuthURL, ovDFMdtURL, ovDFReplyURL, skuInfoURL, downloadLinkURL = origPages, origAuth, origMdt, origReply, origSku, origLink
	})
}

func defaultVariant() provider.Variant {
	return Windows11{}.Variants()[0]
}

func arm64Variant() provider.Variant {
	return Windows11{}.Variants()[1]
}

func TestWindows11Check(t *testing.T) {
	withTestServer(t, false)

	res, err := Windows11{}.Check(context.Background(), defaultVariant())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if res.Version != "25H2" {
		t.Errorf("Check() version = %q, want %q", res.Version, "25H2")
	}
}

func TestWindows11Download(t *testing.T) {
	withTestServer(t, false)
	dir := t.TempDir()

	result, err := Windows11{}.Download(context.Background(), defaultVariant(), provider.CheckResult{Version: "25H2"}, dir, nil)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if result.Filename != "windows-11-25H2-x64.iso" {
		t.Errorf("Download() filename = %q", result.Filename)
	}
	if _, err := os.Stat(filepath.Join(dir, result.Filename)); err != nil {
		t.Errorf("downloaded file missing: %v", err)
	}
}

func TestWindows11DownloadSentinelRejectionIsClear(t *testing.T) {
	withTestServer(t, true)

	_, err := Windows11{}.Download(context.Background(), defaultVariant(), provider.CheckResult{Version: "25H2"}, t.TempDir(), nil)
	if err == nil {
		t.Fatal("Download() expected a rejection error, got nil")
	}
	if !strings.Contains(err.Error(), "residential") {
		t.Errorf("rejection error should explain the residential-IP requirement, got: %v", err)
	}
}

// TestWindows11CompletesOVDFHandshake is a regression test for a real
// report: Microsoft's Sentinel anti-bot kept rejecting the download even
// with the vlscppe.microsoft.com session-authorization step working and
// cookies correctly persisted. Root cause, traced against other currently-
// working implementations of this same flow: a second anti-fraud handshake
// against ov-df.microsoft.com was missing entirely. Confirms Download
// fetches the challenge token/ticks from mdt.js and echoes them back
// correctly (session_id, CustomerId, w, and rticks all present and
// matching) rather than skipping the step or sending garbage.
func TestWindows11CompletesOVDFHandshake(t *testing.T) {
	var replyQuery string

	mux := http.NewServeMux()
	mux.HandleFunc("/software-download/windows11", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<select><option value="3321">Windows 11 (multi-edition ISO for x64 devices)</option></select>`))
	})
	mux.HandleFunc("/tags", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/mdt.js", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`var x = "?w=cafef00d"; rticks="+13579";`))
	})
	mux.HandleFunc("/ovreply", func(w http.ResponseWriter, r *http.Request) {
		replyQuery = r.URL.RawQuery
		w.WriteHeader(200)
	})
	mux.HandleFunc("/skuinfo", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"Skus":[{"Id":"20047","Language":"English International","ProductDisplayName":"Windows 11 25H2__V2"}]}`))
	})
	mux.HandleFunc("/downloadlinks", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ProductDownloadOptions":[{"Uri":"http://` + r.Host + `/iso/x64.iso"}]}`))
	})
	mux.HandleFunc("/iso/x64.iso", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("fake iso"))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	origPages, origAuth, origMdt, origReply, origSku, origLink := downloadPageURLs, sessionAuthURL, ovDFMdtURL, ovDFReplyURL, skuInfoURL, downloadLinkURL
	downloadPageURLs = map[string]string{"x64": srv.URL + "/software-download/windows11"}
	sessionAuthURL = srv.URL + "/tags"
	ovDFMdtURL = srv.URL + "/mdt.js"
	ovDFReplyURL = srv.URL + "/ovreply"
	skuInfoURL = srv.URL + "/skuinfo"
	downloadLinkURL = srv.URL + "/downloadlinks"
	t.Cleanup(func() {
		downloadPageURLs, sessionAuthURL, ovDFMdtURL, ovDFReplyURL, skuInfoURL, downloadLinkURL = origPages, origAuth, origMdt, origReply, origSku, origLink
	})

	_, err := Windows11{}.Download(context.Background(), defaultVariant(), provider.CheckResult{Version: "25H2"}, t.TempDir(), nil)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	q, err := url.ParseQuery(replyQuery)
	if err != nil {
		t.Fatalf("parse ov-df reply query %q: %v", replyQuery, err)
	}
	if q.Get("w") != "cafef00d" {
		t.Errorf("ov-df reply w = %q, want %q", q.Get("w"), "cafef00d")
	}
	if q.Get("rticks") != "13579" {
		t.Errorf("ov-df reply rticks = %q, want %q", q.Get("rticks"), "13579")
	}
	if q.Get("CustomerId") != ovDFInstanceID {
		t.Errorf("ov-df reply CustomerId = %q, want %q", q.Get("CustomerId"), ovDFInstanceID)
	}
	if q.Get("session_id") == "" {
		t.Error("ov-df reply is missing session_id")
	}
	if q.Get("mdt") == "" {
		t.Error("ov-df reply is missing mdt (timestamp)")
	}
}

// TestWindows11SessionCookiePersistsAcrossSteps is a regression test for a
// real report: Microsoft's "Sentinel" anti-bot kept rejecting the download
// step even from a normal residential connection. Root cause: every step of
// this 4-step flow (page load, session-auth, SKU lookup, download-link
// lookup) went out through a bare client with no cookie jar, so a session
// cookie Microsoft set on the very first request was silently dropped
// instead of being carried forward on the later ones — unlike a real
// browser completing the same flow. Confirms the cookie set on the initial
// page load is actually sent back on both subsequent API calls.
func TestWindows11SessionCookiePersistsAcrossSteps(t *testing.T) {
	var sawCookieOnSkuInfo, sawCookieOnDownloadLinks bool

	mux := http.NewServeMux()
	mux.HandleFunc("/software-download/windows11", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "msSession", Value: "abc123", Path: "/"})
		_, _ = w.Write([]byte(`<select><option value="3321">Windows 11 (multi-edition ISO for x64 devices)</option></select>`))
	})
	mux.HandleFunc("/tags", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/mdt.js", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`var x = "?w=deadbeef1234"; rticks="+56789";`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/skuinfo", func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("msSession"); err == nil && c.Value == "abc123" {
			sawCookieOnSkuInfo = true
		}
		_, _ = w.Write([]byte(`{"Skus":[{"Id":"20047","Language":"English International","ProductDisplayName":"Windows 11 25H2__V2"}]}`))
	})
	mux.HandleFunc("/downloadlinks", func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("msSession"); err == nil && c.Value == "abc123" {
			sawCookieOnDownloadLinks = true
		}
		_, _ = w.Write([]byte(`{"ProductDownloadOptions":[{"Uri":"http://` + r.Host + `/iso/x64.iso"}]}`))
	})
	mux.HandleFunc("/iso/x64.iso", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("fake iso"))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	origPages, origAuth, origMdt, origReply, origSku, origLink := downloadPageURLs, sessionAuthURL, ovDFMdtURL, ovDFReplyURL, skuInfoURL, downloadLinkURL
	downloadPageURLs = map[string]string{"x64": srv.URL + "/software-download/windows11"}
	sessionAuthURL = srv.URL + "/tags"
	ovDFMdtURL = srv.URL + "/mdt.js"
	ovDFReplyURL = srv.URL + "/"
	skuInfoURL = srv.URL + "/skuinfo"
	downloadLinkURL = srv.URL + "/downloadlinks"
	t.Cleanup(func() {
		downloadPageURLs, sessionAuthURL, ovDFMdtURL, ovDFReplyURL, skuInfoURL, downloadLinkURL = origPages, origAuth, origMdt, origReply, origSku, origLink
	})

	_, err := Windows11{}.Download(context.Background(), defaultVariant(), provider.CheckResult{Version: "25H2"}, t.TempDir(), nil)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if !sawCookieOnSkuInfo {
		t.Error("session cookie from the initial page load was not sent on the SKU-info step")
	}
	if !sawCookieOnDownloadLinks {
		t.Error("session cookie from the initial page load was not sent on the download-links step")
	}
}

// TestWindows11DownloadFailsClearlyWhenAuthUnreachable is a regression test
// for a real report: Microsoft's Sentinel anti-bot rejected the download
// even from a confirmed residential IP. Root cause: the session-
// authorization request (step 2, vlscppe.microsoft.com in production) was
// unreachable — the user's own router-level DNS blocklist resolved that
// domain (Microsoft's ThreatMetrix anti-fraud endpoint) to 0.0.0.0 — and
// the failure was silently discarded, so Sentinel rejected an incomplete
// session two steps later with no indication why. Confirms Download now
// fails immediately with an actionable message when step 2 is unreachable,
// while Check (which doesn't need Sentinel's approval) still tolerates it.
func TestWindows11DownloadFailsClearlyWhenAuthUnreachable(t *testing.T) {
	withTestServer(t, false)

	// Point the auth step at a closed port so the request fails outright,
	// standing in for the real "blackholed by DNS" failure.
	closedPortURL := "http://127.0.0.1:1"
	origAuth := sessionAuthURL
	sessionAuthURL = closedPortURL
	t.Cleanup(func() { sessionAuthURL = origAuth })

	if _, err := (Windows11{}).Check(context.Background(), defaultVariant()); err != nil {
		t.Errorf("Check() error = %v, want nil (Check shouldn't need session authorization to succeed)", err)
	}

	_, err := Windows11{}.Download(context.Background(), defaultVariant(), provider.CheckResult{Version: "25H2"}, t.TempDir(), nil)
	if err == nil {
		t.Fatal("Download() error = nil, want a clear session-authorization failure")
	}
	if !strings.Contains(err.Error(), "session authorization failed") {
		t.Errorf("Download() error = %v, want it to explain the session-authorization failure", err)
	}
}

// serveFakeDNS answers every A query on pc with 127.0.0.1 and every AAAA
// query with an empty (NOERROR/NODATA) answer, standing in for a public
// resolver like 1.1.1.1 that correctly resolves a hostname the system/local
// resolver has blackholed. Stops when pc is closed.
func serveFakeDNS(pc net.PacketConn) {
	buf := make([]byte, 512)
	for {
		n, addr, err := pc.ReadFrom(buf)
		if err != nil {
			return
		}
		var msg dnsmessage.Message
		if err := msg.Unpack(buf[:n]); err != nil || len(msg.Questions) == 0 {
			continue
		}
		q := msg.Questions[0]

		resp := dnsmessage.Message{
			Header:    dnsmessage.Header{ID: msg.ID, Response: true, RCode: dnsmessage.RCodeSuccess},
			Questions: []dnsmessage.Question{q},
		}
		if q.Type == dnsmessage.TypeA {
			resp.Answers = []dnsmessage.Resource{{
				Header: dnsmessage.ResourceHeader{Name: q.Name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET, TTL: 60},
				Body:   &dnsmessage.AResource{A: [4]byte{127, 0, 0, 1}},
			}}
		}

		packed, err := resp.Pack()
		if err != nil {
			continue
		}
		_, _ = pc.WriteTo(packed, addr)
	}
}

// TestDialWithDNSBypassFallsBackToPublicDNS is a regression test for a real
// report: a user's router-level DNS blocklist resolved
// vlscppe.microsoft.com to 0.0.0.0, breaking the session-authorization
// step. A hostname that the "try the system resolver first" attempt can't
// connect through must still succeed once resolved against a fallback DNS
// server, exactly as it would for a blackholed real domain.
func TestDialWithDNSBypassFallsBackToPublicDNS(t *testing.T) {
	realSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer realSrv.Close()
	_, realPortStr, err := net.SplitHostPort(strings.TrimPrefix(realSrv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	realPort, err := strconv.Atoi(realPortStr)
	if err != nil {
		t.Fatal(err)
	}

	fakeDNS, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer fakeDNS.Close()
	go serveFakeDNS(fakeDNS)

	origServers := dnsBypassServers
	dnsBypassServers = []string{fakeDNS.LocalAddr().String()}
	t.Cleanup(func() { dnsBypassServers = origServers })

	// blocked.example.invalid isn't a real hostname, so the "try the system
	// resolver first" attempt fails outright — the same effective outcome
	// as a real domain blackholed to 0.0.0.0: no working connection without
	// the fallback.
	conn, err := dialWithDNSBypass(context.Background(), "tcp", fmt.Sprintf("blocked.example.invalid:%d", realPort))
	if err != nil {
		t.Fatalf("dialWithDNSBypass() error = %v, want success via the fallback DNS server", err)
	}
	_ = conn.Close()
}

func TestWindows11LocalVersion(t *testing.T) {
	version, ok := (Windows11{}).LocalVersion("windows-11-25H2-x64.iso", defaultVariant())
	if !ok || version != "25H2" {
		t.Errorf("LocalVersion() = (%q, %v), want (\"25H2\", true)", version, ok)
	}
}

// TestWindows11Arm64 covers the second variant: Microsoft splits x64 and
// Arm64 consumer ISOs across two separate download pages (each with its
// own product-edition id), not two options on one page — Check/Download
// must hit the right page and pick the right ProductDownloadOptions entry.
func TestWindows11Arm64(t *testing.T) {
	withTestServer(t, false)

	res, err := Windows11{}.Check(context.Background(), arm64Variant())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if res.Version != "25H2" {
		t.Errorf("Check() version = %q, want %q", res.Version, "25H2")
	}

	dir := t.TempDir()
	result, err := Windows11{}.Download(context.Background(), arm64Variant(), provider.CheckResult{Version: "25H2"}, dir, nil)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if result.Filename != "windows-11-25H2-arm64.iso" {
		t.Errorf("Download() filename = %q", result.Filename)
	}

	version, ok := (Windows11{}).LocalVersion("Win11_25H2_EnglishInternational_Arm64_v2.iso", arm64Variant())
	if !ok || version != "25H2" {
		t.Errorf("LocalVersion() = (%q, %v), want (\"25H2\", true)", version, ok)
	}
}

// TestWindows11LocalVersionOfficialFilename is a regression test for a real
// report: a user's manually-downloaded ISOs use Microsoft's own filename
// convention, not this app's generated one, and were showing "Not found"
// despite being present on disk.
func TestWindows11LocalVersionOfficialFilename(t *testing.T) {
	cases := []struct {
		filename string
		want     string
		ok       bool
	}{
		{"Win11_23H2_EnglishInternational_x64v2.iso", "23H2", true},
		{"Win11_23H2_French_x64v2 (1).iso", "", false}, // different language edition
		{"Win11_24H2_EnglishInternational_x64v1.iso", "24H2", true},
	}
	for _, c := range cases {
		version, ok := (Windows11{}).LocalVersion(c.filename, defaultVariant())
		if ok != c.ok || version != c.want {
			t.Errorf("LocalVersion(%q) = (%q, %v), want (%q, %v)", c.filename, version, ok, c.want, c.ok)
		}
	}
}
