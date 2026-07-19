// Package windows11 checks and downloads the consumer, multi-edition
// Windows 11 ISO ("any language" — this provider defaults to English
// International, x64) directly from Microsoft's software-download site.
//
// Unlike every other provider, there is no static download URL: Microsoft
// gates the real link behind a short-lived, session-based flow, plus two
// anti-fraud handshakes real reports traced actual Sentinel rejections
// back to:
//
//  1. GET the software-download page, scrape the numeric product-edition id.
//  2. "Authorize" a random session UUID by hitting vlscppe.microsoft.com
//     (Microsoft's ThreatMetrix anti-fraud endpoint).
//  3. Complete a second anti-fraud handshake against ov-df.microsoft.com:
//     fetch a challenge token, then echo it back with a timestamp.
//  4. GetSkuInformationByProductEdition -> map the requested language to a
//     SKU id, and read the release name (e.g. "Windows 11 25H2") for the
//     version.
//  5. GetProductDownloadLinksBySku -> the actual per-architecture ISO URL,
//     valid for ~24h.
//
// Step 5 (Download) is guarded by Microsoft's "Sentinel" anti-bot, most
// commonly known to reject datacenter/cloud IPs — but two confirmed real
// cases rejected a genuine residential IP too:
//   - Step 2 silently swallowed a failure to reach vlscppe.microsoft.com —
//     confirmed via a DNS lookup returning a real IP from public resolvers
//     but 0.0.0.0 from the user's own router (a common ad/tracker DNS
//     blocklist entry) — so the session Sentinel evaluated later had never
//     actually completed that fraud check and got rejected regardless of IP
//     reputation. This app now falls back to a public DNS resolver for this
//     domain specifically (see dialWithDNSBypass) and surfaces a failure
//     here immediately instead of swallowing it.
//   - Step 3 (the ov-df handshake) was missing entirely until traced
//     against other currently-working implementations of this same flow —
//     without it, Sentinel rejected the final request even with steps 1-2
//     and cookie persistence all working correctly.
package windows11

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/net/publicsuffix"

	"github.com/arnaudcharles/iso-auto-downloader/internal/download"
	"github.com/arnaudcharles/iso-auto-downloader/internal/provider"
)

// These endpoints are vars (not consts) so tests can point them at an
// httptest server. profileID/orgID are Microsoft's fixed public constants
// for this flow.
var (
	// downloadPageURLs is keyed by architecture: Microsoft splits the x64
	// and Arm64 consumer ISOs across two separate pages (each with its own
	// product-edition id), not two options on one page.
	downloadPageURLs = map[string]string{
		"x64":   "https://www.microsoft.com/en-us/software-download/windows11",
		"arm64": "https://www.microsoft.com/en-us/software-download/windows11arm64",
	}
	sessionAuthURL  = "https://vlscppe.microsoft.com/tags"
	ovDFMdtURL      = "https://ov-df.microsoft.com/mdt.js"
	ovDFReplyURL    = "https://ov-df.microsoft.com/"
	skuInfoURL      = "https://www.microsoft.com/software-download-connector/api/getskuinformationbyproductedition"
	downloadLinkURL = "https://www.microsoft.com/software-download-connector/api/GetProductDownloadLinksBySku"

	profileID = "606624d44113"
	orgID     = "y6jn8c31"

	// ovDFInstanceID is a fixed constant Microsoft's own ov-df.microsoft.com
	// challenge/response step expects (same value every known-working
	// implementation of this flow uses; not something derived per session).
	ovDFInstanceID = "560dc9f3-1aa5-4a2f-b63c-9e18f8d0e175"

	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:134.0) Gecko/20100101 Firefox/134.0"
)

var productEditionRe = regexp.MustCompile(`<option value="([0-9]+)">Windows`)

// ovDFTokenRe and ovDFTicksRe extract the "w" token and "rticks" value from
// the mdt.js response body of Microsoft's ov-df.microsoft.com challenge.
var ovDFTokenRe = regexp.MustCompile(`[?&]w=([A-Fa-f0-9]+)`)
var ovDFTicksRe = regexp.MustCompile(`rticks="\+?(\d+)`)

// releaseNameRe captures the release channel (e.g. "25H2") from a SKU's
// ProductDisplayName. x64's is "Windows 11 25H2__V2"; Arm64's inserts the
// architecture in between: "Windows 11 Arm64 25H2__V2" — .*? skips
// whatever's there non-greedily rather than assuming the version comes
// right after "Windows 11".
var releaseNameRe = regexp.MustCompile(`Windows 11.*?(\d+H\d+)`)

type Windows11 struct{}

func init() {
	provider.Register(Windows11{})
}

func (Windows11) ID() string                  { return "windows11" }
func (Windows11) Name() string                { return "Windows 11" }
func (Windows11) Category() provider.Category { return provider.CategoryWindows }

func (Windows11) Variants() []provider.Variant {
	return []provider.Variant{
		{Edition: "consumer", Arch: "x64", Lang: "English International"},
		{Edition: "consumer", Arch: "arm64", Lang: "English International"},
	}
}

type skuInfo struct {
	Skus []struct {
		ID                 string `json:"Id"`
		Language           string `json:"Language"`
		ProductDisplayName string `json:"ProductDisplayName"`
	} `json:"Skus"`
}

type downloadLinks struct {
	Errors []struct {
		Value string `json:"Value"`
	} `json:"Errors"`
	ProductDownloadOptions []struct {
		URI string `json:"Uri"`
	} `json:"ProductDownloadOptions"`
}

// dnsBypassServers are public DNS resolvers tried, in order, to look up a
// hostname directly when the system resolver's answer doesn't lead to a
// working connection. Real report: a user's router-level DNS blocklist
// resolved vlscppe.microsoft.com (Microsoft's ThreatMetrix anti-fraud
// endpoint — a common ad/tracker blocklist entry despite not actually being
// malicious) to 0.0.0.0, silently breaking this flow's session-
// authorization step. Falling back to a public resolver for just this
// provider's own requests routes around that without requiring the user to
// change their router/DNS configuration.
var dnsBypassServers = []string{"1.1.1.1:53", "8.8.8.8:53"}

// dialWithDNSBypass tries the system resolver first (so a working, unblocked
// setup is left alone) and only falls back to looking the host up directly
// against a public DNS server if that fails to produce a working
// connection — e.g. because the system resolver returned a blackholed
// address like 0.0.0.0, which "resolves" successfully but never connects.
func dialWithDNSBypass(ctx context.Context, network, addr string) (net.Conn, error) {
	dialer := &net.Dialer{}

	if conn, err := dialer.DialContext(ctx, network, addr); err == nil {
		return conn, nil
	}

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for _, server := range dnsBypassServers {
		resolver := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return dialer.DialContext(ctx, network, server)
			},
		}
		ips, err := resolver.LookupIPAddr(ctx, host)
		if err != nil || len(ips) == 0 {
			lastErr = err
			continue
		}
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("windows11: could not connect to %s, including via a public DNS fallback: %w", host, lastErr)
}

// newSessionClient returns an http.Client with a cookie jar, scoped to one
// Check/Download call. Real report: Microsoft's "Sentinel" anti-bot kept
// rejecting the download step even from a normal residential connection —
// every request in this multi-step flow (page load, session-auth, SKU
// lookup, download-link lookup) was going out through a bare client with no
// cookie jar, so any session cookie Microsoft set on an earlier step was
// silently dropped instead of being carried forward, unlike a real browser
// completing the same flow. A fresh jar per call (not a shared/global one)
// keeps concurrent Check/Download calls for different variants from
// cross-contaminating each other's session state. The custom transport adds
// the DNS-bypass dial from above.
func newSessionClient() (*http.Client, error) {
	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return nil, fmt.Errorf("windows11: create cookie jar: %w", err)
	}
	return &http.Client{
		Jar:       jar,
		Transport: &http.Transport{DialContext: dialWithDNSBypass},
	}, nil
}

// httpGet issues a GET matching the header shape of known-working
// implementations of this flow (dockur/windows, ElliotKillick/Mido): a
// browser-like User-Agent, no Accept header at all (Microsoft's endpoints
// are pickier about a populated one than about its absence), and a Referer
// only where referer is non-empty — real report: this used to hardcode
// Referer to the literal string "localhost" on every request, which no real
// browser session completing this flow would ever send.
func httpGet(ctx context.Context, client *http.Client, url, referer string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("windows11: build request for %s: %w", url, err)
	}
	req.Header.Set("User-Agent", userAgent)
	if referer != "" {
		req.Header.Set("Referer", referer)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("windows11: GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("windows11: read body of %s: %w", url, err)
	}
	return string(body), nil
}

// completeOVDFChallenge performs Microsoft's ov-df.microsoft.com anti-fraud
// handshake: fetch a challenge token ("w") and a "rticks" value from
// mdt.js, then echo them back with a timestamp. Neither response body is
// otherwise used.
func completeOVDFChallenge(ctx context.Context, client *http.Client, sessionID string) error {
	challengeURL := fmt.Sprintf("%s?instanceId=%s&PageId=si&session_id=%s", ovDFMdtURL, ovDFInstanceID, sessionID)
	body, err := httpGet(ctx, client, challengeURL, "")
	if err != nil {
		return err
	}

	tm := ovDFTokenRe.FindStringSubmatch(body)
	rm := ovDFTicksRe.FindStringSubmatch(body)
	if tm == nil || rm == nil {
		return fmt.Errorf("could not extract the ov-df challenge token/ticks from %s", ovDFMdtURL)
	}

	select {
	case <-time.After(200 * time.Millisecond):
	case <-ctx.Done():
		return ctx.Err()
	}

	replyURL := fmt.Sprintf("%s?session_id=%s&CustomerId=%s&PageId=si&w=%s&mdt=%d&rticks=%s",
		ovDFReplyURL, sessionID, ovDFInstanceID, tm[1], time.Now().UnixMilli(), rm[1])
	_, err = httpGet(ctx, client, replyURL, "")
	return err
}

// resolveSKU runs steps 1-3: returns the SKU id for lang and the release
// name (version), e.g. "25H2". client must be the same cookie-jar-backed
// client used for every step of the same session (see newSessionClient).
//
// requireAuth controls whether a failure to reach the session-authorization
// endpoint (step 2) is fatal. Check() doesn't need Sentinel's approval just
// to read the current version, so it tolerates that endpoint being
// unreachable the way it always has; Download() does need it — see the
// call site below for why.
func resolveSKU(ctx context.Context, client *http.Client, sessionID, arch, lang string, requireAuth bool) (skuID, version string, err error) {
	pageURL, ok := downloadPageURLs[arch]
	if !ok {
		return "", "", fmt.Errorf("windows11: unsupported architecture %q", arch)
	}
	page, err := httpGet(ctx, client, pageURL, "")
	if err != nil {
		return "", "", err
	}
	pm := productEditionRe.FindStringSubmatch(page)
	if pm == nil {
		return "", "", fmt.Errorf("windows11: could not find product edition id on the download page")
	}
	productEditionID := pm[1]

	// Authorize the session — the response body is irrelevant, but reaching
	// this endpoint at all matters: it's Microsoft's ThreatMetrix anti-fraud
	// check, and Sentinel rejects the later download-link request if this
	// session never actually completed it, regardless of IP reputation.
	// Real report: this failed silently (error discarded) because
	// vlscppe.microsoft.com was blackholed by the user's own router-level
	// DNS ad/tracker blocklist (confirmed: public resolvers returned a real
	// IP, the router's resolver returned 0.0.0.0) — surfacing it now (when
	// it actually matters, i.e. requireAuth) saves re-diagnosing the same
	// thing from a confusing Sentinel rejection two steps later.
	if _, authErr := httpGet(ctx, client, fmt.Sprintf("%s?org_id=%s&session_id=%s", sessionAuthURL, orgID, sessionID), ""); authErr != nil && requireAuth {
		return "", "", fmt.Errorf("windows11: session authorization failed, which Microsoft's Sentinel anti-bot will reject downstream regardless of IP reputation (%w) — if this is a DNS error, check whether your router/DNS blocks vlscppe.microsoft.com (Microsoft's ThreatMetrix anti-fraud endpoint, a common ad/tracker blocklist entry)", authErr)
	}

	// A second, separate anti-fraud handshake against ov-df.microsoft.com —
	// missing entirely until a real report traced a persistent Sentinel
	// rejection (even with the vlscppe step above working and cookies
	// correctly persisted) to this exact gap, confirmed against other
	// currently-working implementations of this flow. Step A fetches a
	// challenge token; step B echoes it back with a timestamp. Response
	// bodies are otherwise irrelevant.
	if authErr := completeOVDFChallenge(ctx, client, sessionID); authErr != nil && requireAuth {
		return "", "", fmt.Errorf("windows11: ov-df anti-fraud handshake failed, which Microsoft's Sentinel anti-bot will reject downstream regardless of IP reputation (%w)", authErr)
	}

	skuURL := fmt.Sprintf("%s?profile=%s&ProductEditionId=%s&SKU=undefined&friendlyFileName=undefined&Locale=en-US&sessionID=%s",
		skuInfoURL, profileID, productEditionID, sessionID)
	skuBody, err := httpGet(ctx, client, skuURL, pageURL)
	if err != nil {
		return "", "", err
	}

	var info skuInfo
	if err := json.Unmarshal([]byte(skuBody), &info); err != nil {
		return "", "", fmt.Errorf("windows11: parse SKU information: %w", err)
	}
	for _, s := range info.Skus {
		if s.Language == lang {
			ver := ""
			if m := releaseNameRe.FindStringSubmatch(s.ProductDisplayName); m != nil {
				ver = m[1]
			}
			return s.ID, ver, nil
		}
	}
	return "", "", fmt.Errorf("windows11: language %q not offered by Microsoft", lang)
}

func (Windows11) Check(ctx context.Context, v provider.Variant) (provider.CheckResult, error) {
	client, err := newSessionClient()
	if err != nil {
		return provider.CheckResult{}, err
	}
	_, version, err := resolveSKU(ctx, client, uuid.NewString(), v.Arch, v.Lang, false)
	if err != nil {
		return provider.CheckResult{}, err
	}
	if version == "" {
		return provider.CheckResult{}, fmt.Errorf("windows11: could not determine release version")
	}
	return provider.CheckResult{Version: version}, nil
}

func (Windows11) Download(ctx context.Context, v provider.Variant, res provider.CheckResult, destDir string, progress provider.ProgressFunc) (provider.DownloadResult, error) {
	client, err := newSessionClient()
	if err != nil {
		return provider.DownloadResult{}, err
	}
	sessionID := uuid.NewString()
	skuID, _, err := resolveSKU(ctx, client, sessionID, v.Arch, v.Lang, true)
	if err != nil {
		return provider.DownloadResult{}, err
	}

	linkURL := fmt.Sprintf("%s?profile=%s&ProductEditionId=undefined&SKU=%s&friendlyFileName=undefined&Locale=en-US&sessionID=%s",
		downloadLinkURL, profileID, skuID, sessionID)
	linkBody, err := httpGet(ctx, client, linkURL, downloadPageURLs[v.Arch])
	if err != nil {
		return provider.DownloadResult{}, err
	}

	var links downloadLinks
	if err := json.Unmarshal([]byte(linkBody), &links); err != nil {
		return provider.DownloadResult{}, fmt.Errorf("windows11: parse download links: %w", err)
	}
	if len(links.Errors) > 0 {
		// The session-authorization step above already succeeded (or this
		// user doesn't have requireAuth-worthy DNS blocking), so this
		// rejection is Microsoft's Sentinel objecting to something else —
		// most commonly a datacenter/VPN IP, which this flow can't work
		// around.
		return provider.DownloadResult{}, fmt.Errorf("windows11: Microsoft rejected the download request (%s) — this step only works from a normal residential connection, not a server/VPN/datacenter IP", links.Errors[0].Value)
	}

	// Case-insensitive match: real report confirmed Microsoft's actual
	// download URI capitalizes the architecture ("Arm64", e.g.
	// ".../Win11_25H2_EnglishInternational_Arm64_v2.iso") while this
	// provider's own Variant.Arch is lowercase ("arm64") — a plain
	// strings.Contains never matched, so arm64 always failed with "no
	// download option returned" even on a request Sentinel had actually
	// accepted.
	fileURL := ""
	for _, opt := range links.ProductDownloadOptions {
		if strings.Contains(strings.ToLower(opt.URI), strings.ToLower(v.Arch)) {
			fileURL = opt.URI
			break
		}
	}
	if fileURL == "" {
		return provider.DownloadResult{}, fmt.Errorf("windows11: no %s download option returned", v.Arch)
	}

	filename := fmt.Sprintf("windows-11-%s-%s.iso", res.Version, v.Arch)
	dest := filepath.Join(destDir, filename)
	size, err := download.Download(ctx, download.Options{
		URL:      fileURL,
		DestPath: dest,
		Progress: download.ProgressFunc(progress),
	})
	if err != nil {
		return provider.DownloadResult{}, err
	}
	return provider.DownloadResult{Path: dest, Filename: filename, SizeBytes: size}, nil
}

// Checksum: Microsoft's consumer download flow doesn't return one alongside
// the signed download link, so there's nothing to verify against.
func (Windows11) Checksum(context.Context, provider.Variant, string) (*download.Checksum, error) {
	return nil, nil
}

// officialFilenameRe matches Microsoft's own official filename convention
// for a manually downloaded consumer ISO, e.g.
// "Win11_23H2_EnglishInternational_x64v2.iso" (x64) or
// "Win11_25H2_EnglishInternational_Arm64_v2.iso" (Arm64 — Microsoft inserts
// an extra underscore before the "v<n>" suffix on this architecture), or,
// when a browser saved a second copy, "...x64v2 (1).iso". This is what a
// real user actually has on disk — this app's own generated name (matched
// below) is only ever seen for files this app downloaded itself.
func officialFilenameRe(v provider.Variant) *regexp.Regexp {
	lang := regexp.QuoteMeta(strings.ReplaceAll(v.Lang, " ", ""))
	arch := regexp.QuoteMeta(v.Arch)
	return regexp.MustCompile(fmt.Sprintf(`(?i)^win11_(\d+h\d+)_%s_%s(?:_?v\d+)?(?:\s*\(\d+\))?\.iso$`, lang, arch))
}

func (Windows11) LocalVersion(filename string, v provider.Variant) (string, bool) {
	if m := officialFilenameRe(v).FindStringSubmatch(filename); m != nil {
		return m[1], true
	}
	re := regexp.MustCompile(fmt.Sprintf(`^windows-11-(\w+)-%s\.iso$`, regexp.QuoteMeta(v.Arch)))
	if m := re.FindStringSubmatch(filename); m != nil {
		return m[1], true
	}
	return "", false
}
