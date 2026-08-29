package instance

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"
)

// The response headers this server sets on everything it answers, and the
// Content Security Policy it sets on the console document.
//
// The policy is cheap here because the zero-CDN rule already paid for it
// (ADR-0045 §5): every asset the console loads is vendored and served from
// this origin, so `default-src 'self'` describes what the bundle already
// is rather than constraining it. The one thing that is not a file is the
// theme resolver in console/index.html, which runs before the first paint
// so a reader who chose light never sees a dark frame. It is admitted by
// hash, computed here from the document that is about to be served, so a
// build that changes the resolver changes the policy with it and neither
// can drift from the other.

const (
	// contentTypeOptions stops a browser guessing a type this server did
	// not send. Every route here sets its own Content-Type.
	contentTypeOptions = "nosniff"

	// referrerPolicy sends no referrer anywhere. Nothing this console
	// links to needs one, and a console URL names the estate it governs.
	referrerPolicy = "no-referrer"

	// strictTransport is one year, and deliberately without
	// includeSubDomains: this process knows the URL it is reached at and
	// nothing about what else the operator runs under that domain, so it
	// is not the thing that should decide for the rest of it.
	strictTransport = "max-age=31536000"
)

// secure sets the headers that are wrong to omit and cost nothing to send.
// They go on every answer rather than on the console alone, because the API
// and the probes are served from the same address to the same browser.
//
// Strict-Transport-Security is sent only where the external URL is https,
// because that is the only place it means anything: the process holds no
// certificate and TLS terminates in front (ADR-0067 §5), and a deployment
// on plain HTTP that told a browser to refuse plain HTTP would have locked
// itself out.
func (s *Server) secure(next http.Handler) http.Handler {
	transport := servedOverTLS(s.cfg.ExternalURL)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("X-Content-Type-Options", contentTypeOptions)
		header.Set("Referrer-Policy", referrerPolicy)
		if transport {
			header.Set("Strict-Transport-Security", strictTransport)
		}
		next.ServeHTTP(w, r)
	})
}

// servedOverTLS reports whether the address a browser reaches this instance
// at is https. An unparseable URL never reaches here: New refuses one.
func servedOverTLS(external string) bool {
	parsed, err := url.Parse(external)
	return err == nil && parsed.Scheme == "https"
}

// contentSecurityPolicy is the policy served with the console document,
// built from that document so the inline scripts in it are the inline
// scripts it admits.
//
// The directives that are not `default-src` are the ones that decide
// something: script-src carries the hashes, style-src widens the default,
// and object-src, base-uri, form-action and frame-ancestors are each
// outside what default-src covers or worth saying twice.
//
// style-src admits inline styles, and that is the one loosening here. The
// bundle sets element styles from JavaScript throughout, and its dialogs
// inject a stylesheet while they are open; a policy that forbade both
// would break the console rather than harden it. Scripts are where the
// defence is, and scripts take no 'unsafe-inline'.
func contentSecurityPolicy(document []byte) string {
	script := []string{"'self'"}
	for _, body := range inlineScripts(document) {
		sum := sha256.Sum256(body)
		script = append(script, "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'")
	}
	return strings.Join([]string{
		"default-src 'self'",
		"script-src " + strings.Join(script, " "),
		"style-src 'self' 'unsafe-inline'",
		"object-src 'none'",
		"base-uri 'none'",
		"form-action 'self'",
		"frame-ancestors 'none'",
	}, "; ")
}

// inlineScripts returns the body of every script element in a document
// that carries no src attribute, byte for byte, which is what a hash is
// taken over.
//
// This reads one document: the index.html a vite build wrote, whose script
// elements are plain and whose attribute values hold no angle bracket.
// A full HTML parser would buy nothing here and would be one more thing
// between the bytes served and the bytes hashed.
func inlineScripts(document []byte) [][]byte {
	var bodies [][]byte
	rest := document
	for {
		start := indexFold(rest, "<script")
		if start < 0 {
			return bodies
		}
		rest = rest[start+len("<script"):]
		open := bytes.IndexByte(rest, '>')
		if open < 0 {
			return bodies
		}
		attributes, body := rest[:open], rest[open+1:]
		end := indexFold(body, "</script")
		if end < 0 {
			return bodies
		}
		rest = body[end:]
		if !namesSrc(attributes) {
			bodies = append(bodies, body[:end])
		}
	}
}

// namesSrc reports whether an open tag's attributes name src, which is
// what separates a script that loads a file from one whose body is in the
// document.
func namesSrc(attributes []byte) bool {
	text := strings.ToLower(string(attributes))
	for i := 0; i+len("src") <= len(text); i++ {
		if text[i:i+len("src")] != "src" {
			continue
		}
		if i > 0 && !attributeBreak(text[i-1]) {
			continue
		}
		if strings.HasPrefix(strings.TrimLeft(text[i+len("src"):], " \t\r\n"), "=") {
			return true
		}
	}
	return false
}

// attributeBreak reports whether a byte can precede an attribute name.
func attributeBreak(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r' || b == '\n' || b == '/'
}

// indexFold is bytes.Index without case sensitivity, because a tag name is
// not case sensitive and a hash taken over the wrong half of a document is
// a policy that blocks the console.
func indexFold(haystack []byte, needle string) int {
	return bytes.Index(bytes.ToLower(haystack), []byte(strings.ToLower(needle)))
}
