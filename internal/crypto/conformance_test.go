package crypto

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
	"testing"
	"unicode/utf16"
)

// Conformance vectors V1-V7 from the Nylon Pay SDK spec, Transport Contract
// section "Conformance vectors". They are generated from the reference
// implementation and verified against the backend's own verifier, so they are
// the ground truth for whether this SDK can talk to the backend at all.
//
// Spec requirement S19. Every vector isolates one failure mode that is
// otherwise diagnosed only as an opaque "auth" error.
//
// Payloads are held as raw JSON text and parsed at test time rather than being
// transcribed into Go literals, so the fixtures cannot drift from the spec.
const (
	conformanceSecret      = "nps_test_conformance_secret"
	conformanceFingerprint = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	conformanceNonce       = "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6"
	conformanceTimestamp   = "1718976000000"
)

type conformanceVector struct {
	id        string
	why       string
	payload   string
	canonical string
	signature string
}

var conformanceVectors = []conformanceVector{
	{
		id:        "V1",
		why:       "a representative payload",
		payload:   `{"amount":5000,"currency":"UGX","customer":{"name":"John Doe","phoneNumber":"+256700000000"},"description":"Test payment","reference":"ORDER-2026-001","metadata":{"orderId":"12345","items":"3"}}`,
		canonical: `{"amount":5000,"currency":"UGX","customer":{"name":"John Doe","phoneNumber":"+256700000000"},"description":"Test payment","metadata":{"items":"3","orderId":"12345"},"reference":"ORDER-2026-001"}`,
		signature: "dc6e1717d7c37d7a3b334087d9882c07663edb2dfc8f2f06cd77c0d2d8a58686",
	},
	{
		id:        "V2",
		why:       "key insertion order is irrelevant: same fields as V1, inserted in reverse",
		payload:   `{"metadata":{"items":"3","orderId":"12345"},"reference":"ORDER-2026-001","description":"Test payment","customer":{"phoneNumber":"+256700000000","name":"John Doe"},"currency":"UGX","amount":5000}`,
		canonical: `{"amount":5000,"currency":"UGX","customer":{"name":"John Doe","phoneNumber":"+256700000000"},"description":"Test payment","metadata":{"items":"3","orderId":"12345"},"reference":"ORDER-2026-001"}`,
		signature: "dc6e1717d7c37d7a3b334087d9882c07663edb2dfc8f2f06cd77c0d2d8a58686",
	},
	{
		id:        "V3",
		why:       "arrays keep their order; objects inside arrays are still sorted",
		payload:   `{"items":[{"unitPrice":2000,"name":"Zeta","quantity":1},{"name":"Alpha","quantity":2,"unitPrice":500}],"tags":["b","a","c"],"amount":4500}`,
		canonical: `{"amount":4500,"items":[{"name":"Zeta","quantity":1,"unitPrice":2000},{"name":"Alpha","quantity":2,"unitPrice":500}],"tags":["b","a","c"]}`,
		signature: "98478585cf5ce0193a9aa6a6e86ff7f5dfc025945d03547dd548b9356b797e4b",
	},
	{
		id:        "V4",
		why:       `string escaping: "/", "<", ">", "&" and non-ASCII stay literal`,
		payload:   `{"note":"café / 50% <b>&\"quoted\"</b>","path":"a/b/c","backslash":"x\\y","newline":"line1\nline2\ttab"}`,
		canonical: `{"backslash":"x\\y","newline":"line1\nline2\ttab","note":"café / 50% <b>&\"quoted\"</b>","path":"a/b/c"}`,
		signature: "80eb3c6e35b8b3dcc67a57e056634b6f68f2f84b9454bea3aa5e86647eb47649",
	},
	{
		id:        "V5",
		why:       "ASCII key ordering: digits before uppercase before underscore before lowercase",
		payload:   `{"Z":1,"_x":2,"a":3,"A":4,"z":5,"0":6}`,
		canonical: `{"0":6,"A":4,"Z":1,"_x":2,"a":3,"z":5}`,
		signature: "7b9da2fccf0140a7b721715b7b61f17ad3407bd40659b54d983c8e8379108adc",
	},
	{
		id:        "V6",
		why:       "empty containers and zero: an empty map is {} and never []",
		payload:   `{"emptyObject":{},"emptyArray":[],"emptyString":"","zero":0}`,
		canonical: `{"emptyArray":[],"emptyObject":{},"emptyString":"","zero":0}`,
		signature: "f1d8a628663cc9279c675b001e5142e10c6880c2713145f7ebb946c73af2e875",
	},
	{
		id:        "V7",
		why:       "non-ASCII key ordering: passes only under true UTF-16 code-unit order",
		payload:   `{"ÿ":1,"Ā":2,"a":3,"注文":4}`,
		canonical: `{"a":3,"ÿ":1,"Ā":2,"注文":4}`,
		signature: "f43182515649622666b920ac1274d6be5ee395d7c295a4eab6e914a48b212a3a",
	},
}

// parseVectorPayload decodes vector JSON text preserving numeric literals, so
// an integer is never widened through float64 on its way to the canonicalizer.
func parseVectorPayload(t *testing.T, text string) any {
	t.Helper()

	decoder := json.NewDecoder(bytes.NewReader([]byte(text)))
	decoder.UseNumber()

	var payload any
	if err := decoder.Decode(&payload); err != nil {
		t.Fatalf("vector payload is not valid JSON: %v", err)
	}
	return payload
}

// TestS19_ConformanceVectors pins both halves of every vector: the canonical
// string and the hex signature. The canonical assertion is the useful one when
// this fails, it says exactly which serialization rule is wrong, where the
// signature alone would only say "different".
func TestS19_ConformanceVectors(t *testing.T) {
	for _, vector := range conformanceVectors {
		t.Run(vector.id, func(t *testing.T) {
			payload := parseVectorPayload(t, vector.payload)

			canonical, err := createCanonicalPayload(payload)
			if err != nil {
				t.Fatalf("%s (%s): canonicalization failed: %v", vector.id, vector.why, err)
			}
			if canonical != vector.canonical {
				t.Errorf("%s (%s): canonical string mismatch\n  want: %s\n  got:  %s",
					vector.id, vector.why, vector.canonical, canonical)
			}

			signature, err := CreateSignature(SignatureInput{
				Fingerprint: conformanceFingerprint,
				Nonce:       conformanceNonce,
				Timestamp:   conformanceTimestamp,
				Payload:     payload,
				Secret:      conformanceSecret,
			})
			if err != nil {
				t.Fatalf("%s: signing failed: %v", vector.id, err)
			}
			if signature != vector.signature {
				t.Errorf("%s (%s): signature mismatch\n  want: %s\n  got:  %s",
					vector.id, vector.why, vector.signature, signature)
			}
		})
	}
}

// TestS19_V1AndV2ProduceIdenticalSignatures states V2's actual claim directly:
// two payloads differing only in key insertion order sign identically.
func TestS19_V1AndV2ProduceIdenticalSignatures(t *testing.T) {
	var signatures []string
	for _, vector := range conformanceVectors[:2] {
		signature, err := CreateSignature(SignatureInput{
			Fingerprint: conformanceFingerprint,
			Nonce:       conformanceNonce,
			Timestamp:   conformanceTimestamp,
			Payload:     parseVectorPayload(t, vector.payload),
			Secret:      conformanceSecret,
		})
		if err != nil {
			t.Fatalf("%s: signing failed: %v", vector.id, err)
		}
		signatures = append(signatures, signature)
	}

	if signatures[0] != signatures[1] {
		t.Errorf("V1 and V2 must sign identically, got %s and %s", signatures[0], signatures[1])
	}
}

// TestV7DistinguishesCodeUnitFromLittleEndianOrder guards the guard. V7 only
// proves anything if the two orderings it separates actually differ, so assert
// that they do. Without this, a regression to UTF-16LE byte sorting could look
// like it still passes V7 if the fixture were ever edited.
func TestV7DistinguishesCodeUnitFromLittleEndianOrder(t *testing.T) {
	keys := []string{"ÿ", "Ā", "a", "注文"}

	byCodeUnit := append([]string(nil), keys...)
	sort.Slice(byCodeUnit, func(i, j int) bool {
		return lessByCodeUnit(utf16.Encode([]rune(byCodeUnit[i])), utf16.Encode([]rune(byCodeUnit[j])))
	})

	littleEndianBytes := func(s string) []byte {
		var buf bytes.Buffer
		for _, unit := range utf16.Encode([]rune(s)) {
			buf.WriteByte(byte(unit & 0xFF))
			buf.WriteByte(byte(unit >> 8))
		}
		return buf.Bytes()
	}
	byLittleEndian := append([]string(nil), keys...)
	sort.Slice(byLittleEndian, func(i, j int) bool {
		return bytes.Compare(littleEndianBytes(byLittleEndian[i]), littleEndianBytes(byLittleEndian[j])) < 0
	})

	wantCodeUnit := "a,ÿ,Ā,注文"
	if got := strings.Join(byCodeUnit, ","); got != wantCodeUnit {
		t.Errorf("code-unit order: want %s, got %s", wantCodeUnit, got)
	}

	// The spec calls this out by name: a UTF-16LE byte sort yields Ā, a, 注文, ÿ.
	wantLittleEndian := "Ā,a,注文,ÿ"
	if got := strings.Join(byLittleEndian, ","); got != wantLittleEndian {
		t.Errorf("UTF-16LE byte order: want %s, got %s", wantLittleEndian, got)
	}

	if strings.Join(byCodeUnit, ",") == strings.Join(byLittleEndian, ",") {
		t.Error("V7 proves nothing if code-unit and UTF-16LE orders agree")
	}
}

// TestCanonicalOrdersNonBMPKeysByCodeUnit covers the gap V7 leaves open.
//
// V7's keys are all BMP, where UTF-8 byte order (which is what Go's native
// string comparison gives) and UTF-16 code-unit order agree, so V7 passes even
// under a code-point sort. The two orders only diverge above U+FFFF, where a
// non-BMP character encodes as a surrogate pair starting in U+D800-U+DBFF and
// therefore sorts BELOW U+E000-U+FFFF under code units, and above it under
// code points.
//
// Merchant metadata keys are arbitrary merchant-supplied strings, so an emoji
// key is reachable in production traffic. Without this test nothing in the
// suite would catch a regression to sort.Strings.
func TestCanonicalOrdersNonBMPKeysByCodeUnit(t *testing.T) {
	// U+1F680 encodes as the surrogate pair D83D DE80, so it must sort before
	// U+E000. A code-point sort puts them the other way round.
	payload := map[string]any{"": 1, "\U0001F680": 2}

	canonical, err := createCanonicalPayload(payload)
	if err != nil {
		t.Fatalf("canonicalization failed: %v", err)
	}

	want := "{\"\U0001F680\":2,\"\":1}"
	if canonical != want {
		t.Errorf("non-BMP key ordering is not UTF-16 code-unit order\n  want: %q\n  got:  %q", want, canonical)
	}
}
