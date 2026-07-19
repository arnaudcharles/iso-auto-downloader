// Package scrape holds small helpers shared by native providers that
// resolve versions and checksums from plain HTML directory listings and
// sha256sum-style checksum files, rather than a dedicated JSON API.
package scrape

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

var hrefRe = regexp.MustCompile(`href="([^"]+)"`)

// Resolve issues a HEAD request to url, following redirects, and returns the
// final URL after redirects plus the final response headers. Used to turn a
// redirector link (e.g. a go.microsoft.com/fwlink) into the concrete CDN URL
// and to read metadata like Last-Modified / Content-Disposition without
// downloading the body.
func Resolve(ctx context.Context, url string) (finalURL string, header http.Header, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return "", nil, fmt.Errorf("scrape: build HEAD request for %s: %w", url, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("scrape: HEAD %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("scrape: HEAD %s: unexpected status %s", url, resp.Status)
	}
	return resp.Request.URL.String(), resp.Header, nil
}

// FilenameFromHeader returns the download filename a response advertises,
// preferring the Content-Disposition filename and falling back to "".
func FilenameFromHeader(header http.Header) string {
	cd := header.Get("Content-Disposition")
	if cd == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(cd)
	if err != nil {
		return ""
	}
	return params["filename"]
}

// FetchString GETs url and returns the body as a string.
func FetchString(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("scrape: build request for %s: %w", url, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("scrape: fetch %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("scrape: fetch %s: unexpected status %s", url, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("scrape: read body of %s: %w", url, err)
	}
	return string(body), nil
}

// Hrefs returns every href attribute value found in an HTML page.
func Hrefs(html string) []string {
	matches := hrefRe.FindAllStringSubmatch(html, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, m[1])
	}
	return out
}

// ChecksumForFile looks up filename's hex digest in a sha256sum/md5sum-style
// checksum file, where each line is "<hex>  <name>" or "<hex> *<name>".
// name may be a path relative to the checksum file (e.g. "v8.10/foo.iso.zip"),
// in which case a suffix match against filename is used.
func ChecksumForFile(sumFile, filename string) (string, error) {
	for _, line := range strings.Split(sumFile, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[1], "*")
		name = strings.TrimPrefix(name, "./")
		if name == filename || strings.HasSuffix(name, "/"+filename) {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("scrape: no checksum found for %q", filename)
}

var bsdChecksumRe = regexp.MustCompile(`(?i)^(?:SHA256|SHA512|SHA1|MD5)\s*\(([^)]+)\)\s*=\s*([0-9a-fA-F]+)$`)

// ChecksumForFileBSD looks up filename's hex digest in a BSD-style checksum
// file, where matching lines look like "SHA256 (name) = hex" — the format
// Fedora's PGP-clearsigned CHECKSUM files use.
func ChecksumForFileBSD(sumFile, filename string) (string, error) {
	for _, line := range strings.Split(sumFile, "\n") {
		m := bsdChecksumRe.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		if m[1] == filename {
			return m[2], nil
		}
	}
	return "", fmt.Errorf("scrape: no BSD-style checksum found for %q", filename)
}

// ExtractLabeledSection returns the block of lines under a "### LABEL:"
// header (up to the next blank line), the convention Clonezilla's combined
// CHECKSUMS.TXT file uses to group one hash type's lines together. Returns
// "" if the label isn't found.
func ExtractLabeledSection(text, label string) string {
	lines := strings.Split(text, "\n")
	header := "### " + strings.ToUpper(label) + ":"
	start := -1
	for i, line := range lines {
		if strings.ToUpper(strings.TrimSpace(line)) == header {
			start = i + 1
			break
		}
	}
	if start == -1 {
		return ""
	}

	var section []string
	for _, line := range lines[start:] {
		if strings.TrimSpace(line) == "" {
			break
		}
		section = append(section, line)
	}
	return strings.Join(section, "\n")
}

// MatchVersion runs re against s and returns its first capture group if re
// matched, for the common "does this filename encode a version" check.
func MatchVersion(re *regexp.Regexp, s string) (string, bool) {
	m := re.FindStringSubmatch(s)
	if m == nil {
		return "", false
	}
	return m[1], true
}

var leadingDigitsRe = regexp.MustCompile(`^\d+`)

// LooksLikeVersion reports whether s has at least one dot-separated segment
// starting with a digit — i.e. whether CompareVersions can meaningfully
// compare it against another version string. A provider's LocalVersion
// sometimes has to report a fixed placeholder for a file whose real version
// can't be determined from its filename alone (e.g. hirensbootcdpe, whose
// filename never embeds one); such a placeholder never looks like a
// version, which callers use to decide whether a comparison against it
// means anything at all, rather than just falling out of CompareVersions as
// an accidental "always older" answer.
func LooksLikeVersion(s string) bool {
	for _, part := range strings.Split(s, ".") {
		if leadingDigitsRe.MatchString(part) {
			return true
		}
	}
	return false
}

// versionSegmentValue returns the leading numeric value of a dot-separated
// version segment for magnitude comparison, e.g. "04" -> 4, "25H2" -> 25,
// "H2" -> 0.
func versionSegmentValue(segment string) int {
	n, _ := strconv.Atoi(leadingDigitsRe.FindString(segment))
	return n
}

// CompareVersions compares dot-separated version strings segment by
// segment, e.g. "24.04.1" vs "24.04". Each segment is compared first by its
// leading numeric value — so "25H2" correctly outranks "23H2" instead of
// both parsing as the same non-number and reading as equal — and, if those
// match, as a plain string, so "25H2" still outranks "25H1". Missing
// trailing components are treated as "0". Returns -1, 0, or 1 like
// strings.Compare.
func CompareVersions(a, b string) int {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		var aSeg, bSeg string
		if i < len(as) {
			aSeg = as[i]
		}
		if i < len(bs) {
			bSeg = bs[i]
		}
		if av, bv := versionSegmentValue(aSeg), versionSegmentValue(bSeg); av != bv {
			if av < bv {
				return -1
			}
			return 1
		}
		if aSeg != bSeg {
			return strings.Compare(aSeg, bSeg)
		}
	}
	return 0
}
