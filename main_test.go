package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type client struct {
	t    *testing.T
	c    *http.Client
	base string
}

func (c client) call(method, path string, body any, want int) json.RawMessage {
	c.t.Helper()
	var b io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			c.t.Fatal(err)
		}
		b = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, c.base+"/api/"+path, b)
	if err != nil {
		c.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.c.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		c.t.Fatal(err)
	}
	if res.StatusCode != want {
		c.t.Fatalf("%s %s: want %d, got %d: %s", method, path, want, res.StatusCode, raw)
	}
	return raw
}
func newClient(t *testing.T, s *httptest.Server) client {
	jar, _ := cookiejar.New(nil)
	return client{t, &http.Client{Jar: jar}, s.URL}
}
func fixture(t *testing.T) (*App, *httptest.Server) {
	t.Helper()
	a, err := newApp(filepath.Join(t.TempDir(), "state.json"), true, false)
	if err != nil {
		t.Fatal(err)
	}
	s := httptest.NewServer(a.handler())
	t.Cleanup(s.Close)
	return a, s
}
func object(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestCompleteRentalJourney(t *testing.T) {
	a, s := fixture(t)
	tenant := newClient(t, s)
	owner := newClient(t, s)
	stranger := newClient(t, s)
	anonymous := newClient(t, s)
	tenant.call("POST", "auth/demo", map[string]string{"role": "tenant"}, 200)
	owner.call("POST", "auth/demo", map[string]string{"role": "landlord"}, 200)
	stranger.call("POST", "auth/register", map[string]string{"name": "Other Tenant", "email": "other@example.com", "password": "A-secure-password", "role": "tenant"}, 201)
	anonymous.call("POST", "requests", map[string]string{"listingId": "1", "message": "I am starting an internship in this area."}, 401)
	request := object(t, tenant.call("POST", "requests", map[string]string{"listingId": "1", "message": "I am starting an internship in this area."}, 201))
	rid := request["id"].(string)
	tenant.call("POST", "requests", map[string]string{"listingId": "1", "message": "This is a duplicate request."}, 409)
	tenant.call("GET", "requests/"+rid+"/messages", nil, 403)
	tenant.call("PATCH", "requests/"+rid, map[string]string{"status": "accepted"}, 403)
	stranger.call("PATCH", "requests/"+rid, map[string]string{"status": "accepted"}, 403)
	owner.call("PATCH", "requests/"+rid, map[string]string{"status": "accepted"}, 200)
	owner.call("PATCH", "requests/"+rid, map[string]string{"status": "declined"}, 409)
	tenant.call("POST", "requests/"+rid+"/messages", map[string]string{"text": "Hello! Can I arrange a viewing?"}, 201)
	owner.call("POST", "requests/"+rid+"/messages", map[string]string{"text": "Yes, Saturday works."}, 201)
	stranger.call("GET", "requests/"+rid+"/messages", nil, 403)
	stranger.call("POST", "requests/"+rid+"/messages", map[string]string{"text": "Intrusion"}, 403)
	var messages []Message
	json.Unmarshal(owner.call("GET", "requests/"+rid+"/messages", nil, 200), &messages)
	if len(messages) != 2 || messages[0].SenderID != "demo-tenant" {
		t.Fatal("message history or sender incorrect")
	}
	agreement := map[string]any{"start": "2026-11-01", "end": "2027-02-01", "rent": 12000, "deposit": 24000, "terms": "Utilities paid separately. Thirty day notice period.", "acknowledged": []string{"demo-landlord", "demo-tenant"}, "landlordName": "spoofed"}
	var draft Agreement
	json.Unmarshal(tenant.call("POST", "requests/"+rid+"/agreement", agreement, 201), &draft)
	if len(draft.Acknowledged) != 0 || draft.LandlordName != "Meera Sharma" {
		t.Fatal("server must own agreement identities and acknowledgements")
	}
	stranger.call("GET", "requests/"+rid+"/agreement", nil, 403)
	owner.call("POST", "requests/"+rid+"/agreement", agreement, 409)
	tenant.call("PATCH", "requests/"+rid+"/agreement", map[string]any{}, 200)
	tenant.call("PATCH", "requests/"+rid+"/agreement", map[string]any{}, 200)
	json.Unmarshal(owner.call("PATCH", "requests/"+rid+"/agreement", map[string]any{}, 200), &draft)
	if len(draft.Acknowledged) != 2 {
		t.Fatal("acknowledgements must be unique and bilateral")
	}
	tenant.call("POST", "listings/1/save", map[string]any{}, 200)
	reloaded, err := newApp(a.path, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.state.Messages) != 2 || len(reloaded.state.Agreements) != 1 || len(reloaded.state.Users[1].Saved) != 1 {
		t.Fatal("changes did not survive reopening the store")
	}
	if len(reloaded.state.Requests) != 1 || reloaded.state.Requests[0].Status != "accepted" {
		t.Fatal("request state not persisted")
	}
}

func TestAdsValidationAndAuthentication(t *testing.T) {
	_, s := fixture(t)
	tenant := newClient(t, s)
	owner := newClient(t, s)
	owner.call("POST", "auth/demo", map[string]string{"role": "landlord"}, 200)
	account := map[string]string{"name": "Student Test", "email": "student@example.com", "password": "long-student-password", "role": "tenant"}
	raw := tenant.call("POST", "auth/register", account, 201)
	if bytes.Contains(raw, []byte("salt")) || bytes.Contains(raw, []byte("hash")) {
		t.Fatal("public account leaks password metadata")
	}
	tenant.call("POST", "auth/register", account, 409)
	tenant.call("POST", "auth/logout", map[string]any{}, 200)
	tenant.call("GET", "me", nil, 401)
	tenant.call("POST", "auth/login", map[string]string{"email": account["email"], "password": "wrong"}, 401)
	tenant.call("POST", "auth/login", map[string]string{"email": account["email"], "password": account["password"]}, 200)
	listing := Listing{Title: "Test home near campus", City: "Pune", Area: "Viman Nagar", Address: "14, Campus Road", Type: "Private room", Rent: 9000, Deposit: 18000, Beds: 1, Baths: 1, Size: 450, MinMonths: 1, Available: "2026-11-01", Description: "A furnished room with a quiet study area.", Furnished: true, Amenities: []string{"Wi-Fi", "Kitchen"}}
	tenant.call("POST", "listings", listing, 403)
	listing.Image = "javascript:alert(1)"
	owner.call("POST", "listings", listing, 400)
	listing.Image = ""
	listing.Rent = -1
	owner.call("POST", "listings", listing, 400)
	listing.Rent = 9000
	listing.OwnerID = "spoofed"
	listing.OwnerName = "spoofed"
	result := object(t, owner.call("POST", "listings", listing, 201))
	if result["ownerId"] != "demo-landlord" || result["ownerName"] != "Meera Sharma" {
		t.Fatal("listing owner can be spoofed")
	}
	need := Need{Title: "Room for a research internship", City: "Pune", Budget: 10000, Months: 2, MoveIn: "2026-11-01", Description: "I need a furnished room near my internship office."}
	owner.call("POST", "needs", need, 403)
	need.TenantID = "spoofed"
	res := object(t, tenant.call("POST", "needs", need, 201))
	if res["tenantId"] == "spoofed" {
		t.Fatal("tenant identity can be spoofed")
	}
	owner.call("POST", "listings/no-such-id/save", map[string]any{}, 404)
}

func TestProductionStartsEmptyAndDemoIsDisabled(t *testing.T) {
	a, err := newApp(filepath.Join(t.TempDir(), "state.json"), false, true)
	if err != nil {
		t.Fatal(err)
	}
	s := httptest.NewServer(a.handler())
	defer s.Close()
	c := newClient(t, s)
	c.call("POST", "auth/demo", map[string]string{"role": "landlord"}, 404)
	if len(a.state.Users) != 0 || len(a.state.Listings) != 0 || len(a.state.Needs) != 0 {
		t.Fatal("production must not contain sample advertisements or users")
	}
	c.call("POST", "auth/register", map[string]string{"name": "Owner", "email": "owner@example.com", "password": "long-owner-password", "role": "landlord"}, 201)
	req, _ := http.NewRequest("POST", s.URL+"/api/auth/login", strings.NewReader(`{"email":"owner@example.com","password":"long-owner-password"}`))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	cookies := res.Cookies()
	if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatal("missing secure session cookie attributes")
	}
}

func TestCSRFExpiryAndRateLimit(t *testing.T) {
	a, s := fixture(t)
	c := newClient(t, s)
	req, _ := http.NewRequest("POST", s.URL+"/api/auth/demo", strings.NewReader(`{"role":"tenant"}`))
	req.Header.Set("Origin", "https://untrusted.example")
	res, err := c.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatal("cross-origin mutation allowed")
	}
	c.call("POST", "auth/demo", map[string]string{"role": "tenant"}, 200)
	a.mu.Lock()
	for k, v := range a.state.Sessions {
		v.Expires = v.Expires.AddDate(-1, 0, 0)
		a.state.Sessions[k] = v
	}
	a.mu.Unlock()
	c.call("GET", "me", nil, 401)
	a.mu.Lock()
	a.attempts["login:127.0.0.1"] = attempt{Count: 20, Until: time.Now().Add(time.Hour)}
	a.mu.Unlock()
	c.call("POST", "auth/login", map[string]string{"email": "bad@example.com", "password": "wrong"}, 429)
}

func TestFailedPersistenceRollsBack(t *testing.T) {
	a, s := fixture(t)
	c := newClient(t, s)
	c.call("POST", "auth/demo", map[string]string{"role": "tenant"}, 200)
	a.mu.Lock()
	a.path = filepath.Join(t.TempDir(), "directory")
	os.Mkdir(a.path, 0700)
	a.mu.Unlock()
	c.call("POST", "requests", map[string]string{"listingId": "1", "message": "I am interested in this apartment."}, 500)
	if len(a.state.Requests) != 0 {
		t.Fatal("failed persistence changed in-memory data")
	}
}

func TestDeclinedRequestStaysClosed(t *testing.T) {
	_, s := fixture(t)
	tenant := newClient(t, s)
	owner := newClient(t, s)
	tenant.call("POST", "auth/demo", map[string]string{"role": "tenant"}, 200)
	owner.call("POST", "auth/demo", map[string]string{"role": "landlord"}, 200)
	request := object(t, tenant.call("POST", "requests", map[string]string{"listingId": "2", "message": "I would like to rent this room."}, 201))
	rid := fmt.Sprint(request["id"])
	owner.call("PATCH", "requests/"+rid, map[string]string{"status": "declined"}, 200)
	tenant.call("POST", "requests/"+rid+"/messages", map[string]string{"text": "Hello"}, 403)
	tenant.call("GET", "requests/"+rid+"/agreement", nil, 403)
}
