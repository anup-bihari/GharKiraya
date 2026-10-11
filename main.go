package main

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed web/*
var webFiles embed.FS

type User struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Email string   `json:"email"`
	Role  string   `json:"role"`
	Salt  string   `json:"salt"`
	Hash  string   `json:"hash"`
	Saved []string `json:"saved"`
}
type Listing struct {
	ID          string   `json:"id"`
	OwnerID     string   `json:"ownerId"`
	OwnerName   string   `json:"ownerName"`
	Title       string   `json:"title"`
	City        string   `json:"city"`
	Area        string   `json:"area"`
	Address     string   `json:"address"`
	Type        string   `json:"type"`
	Rent        int      `json:"rent"`
	Deposit     int      `json:"deposit"`
	Beds        int      `json:"beds"`
	Baths       int      `json:"baths"`
	Size        int      `json:"size"`
	MinMonths   int      `json:"minMonths"`
	Available   string   `json:"available"`
	Furnished   bool     `json:"furnished"`
	Amenities   []string `json:"amenities"`
	Image       string   `json:"image"`
	Description string   `json:"description"`
	CreatedAt   string   `json:"createdAt"`
}
type Need struct {
	ID          string `json:"id"`
	TenantID    string `json:"tenantId"`
	TenantName  string `json:"tenantName"`
	Title       string `json:"title"`
	City        string `json:"city"`
	Budget      int    `json:"budget"`
	Months      int    `json:"months"`
	MoveIn      string `json:"moveIn"`
	Description string `json:"description"`
	CreatedAt   string `json:"createdAt"`
}
type Request struct {
	ID           string `json:"id"`
	ListingID    string `json:"listingId"`
	TenantID     string `json:"tenantId"`
	LandlordID   string `json:"landlordId"`
	TenantName   string `json:"tenantName"`
	LandlordName string `json:"landlordName"`
	Title        string `json:"title"`
	Message      string `json:"message"`
	Status       string `json:"status"`
	CreatedAt    string `json:"createdAt"`
}
type Message struct {
	ID         string `json:"id"`
	RequestID  string `json:"requestId"`
	SenderID   string `json:"senderId"`
	SenderName string `json:"senderName"`
	Text       string `json:"text"`
	CreatedAt  string `json:"createdAt"`
}
type Agreement struct {
	ID           string   `json:"id"`
	RequestID    string   `json:"requestId"`
	Title        string   `json:"title"`
	LandlordName string   `json:"landlordName"`
	TenantName   string   `json:"tenantName"`
	Address      string   `json:"address"`
	Rent         int      `json:"rent"`
	Deposit      int      `json:"deposit"`
	Start        string   `json:"start"`
	End          string   `json:"end"`
	Terms        string   `json:"terms"`
	Acknowledged []string `json:"acknowledged"`
	CreatedAt    string   `json:"createdAt"`
}
type Session struct {
	UserID  string    `json:"userId"`
	Expires time.Time `json:"expires"`
}
type State struct {
	Users      []User             `json:"users"`
	Listings   []Listing          `json:"listings"`
	Needs      []Need             `json:"needs"`
	Requests   []Request          `json:"requests"`
	Messages   []Message          `json:"messages"`
	Agreements []Agreement        `json:"agreements"`
	Sessions   map[string]Session `json:"sessions"`
}
type attempt struct {
	Count int
	Until time.Time
}
type App struct {
	mu       sync.Mutex
	state    State
	path     string
	demo     bool
	secure   bool
	attempts map[string]attempt
}
type apiError struct {
	status  int
	message string
}

func (e apiError) Error() string { return e.message }
func bad(message string) error   { return apiError{400, message} }
func forbidden() error           { return apiError{403, "You do not have access to this action."} }
func now() string                { return time.Now().UTC().Format(time.RFC3339) }
func id() string {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func passwordHash(password, salt string) string {
	key, err := pbkdf2.Key(sha256.New, password, []byte(salt), 600000, 32)
	if err != nil {
		panic(err)
	}
	return hex.EncodeToString(key)
}
func sessionKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
func publicUser(u *User) any {
	if u == nil {
		return nil
	}
	return map[string]any{"id": u.ID, "name": u.Name, "email": u.Email, "role": u.Role, "saved": u.Saved}
}
func decode(r *http.Request, dst any) error {
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return bad("Please check the form fields and try again.")
	}
	if err := d.Decode(&struct{}{}); err != io.EOF {
		return bad("Only one JSON object is allowed.")
	}
	return nil
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func (a *App) save() error {
	if err := os.MkdirAll(filepath.Dir(a.path), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(a.state, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(a.path), ".state-*.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(name, a.path)
}
func newApp(path string, demo, secure bool) (*App, error) {
	a := &App{path: path, demo: demo, secure: secure, attempts: map[string]attempt{}}
	b, err := os.ReadFile(path)
	if err == nil {
		if err = json.Unmarshal(b, &a.state); err != nil {
			return nil, fmt.Errorf("read data: %w", err)
		}
	} else if errors.Is(err, os.ErrNotExist) {
		a.seed()
		if err = a.save(); err != nil {
			return nil, err
		}
	} else {
		return nil, err
	}
	if a.state.Sessions == nil {
		a.state.Sessions = map[string]Session{}
	}
	return a, nil
}
func (a *App) user(r *http.Request) *User {
	c, err := r.Cookie("gk_session")
	if err != nil {
		return nil
	}
	s, ok := a.state.Sessions[sessionKey(c.Value)]
	if !ok || time.Now().After(s.Expires) {
		return nil
	}
	for i := range a.state.Users {
		if a.state.Users[i].ID == s.UserID {
			return &a.state.Users[i]
		}
	}
	return nil
}
func (a *App) login(w http.ResponseWriter, u User) {
	token := id() + id()
	expires := time.Now().Add(7 * 24 * time.Hour)
	for k, s := range a.state.Sessions {
		if time.Now().After(s.Expires) {
			delete(a.state.Sessions, k)
		}
	}
	a.state.Sessions[sessionKey(token)] = Session{u.ID, expires}
	http.SetCookie(w, &http.Cookie{Name: "gk_session", Value: token, Path: "/", HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode, MaxAge: 604800, Expires: expires})
}
func (a *App) limited(key string) bool {
	t := time.Now()
	v := a.attempts[key]
	if t.After(v.Until) {
		v = attempt{Until: t.Add(15 * time.Minute)}
	}
	v.Count++
	a.attempts[key] = v
	if len(a.attempts) > 5000 {
		for k, x := range a.attempts {
			if t.After(x.Until) {
				delete(a.attempts, k)
			}
		}
	}
	return v.Count > 20
}
func (a *App) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/", a.api)
	static, _ := fs.Sub(webFiles, "web")
	mux.Handle("/", http.FileServer(http.FS(static)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' https://images.unsplash.com; style-src 'self' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com; script-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		if r.Method != "GET" && r.Method != "HEAD" {
			origin := r.Header.Get("Origin")
			if origin != "" {
				u, err := url.Parse(origin)
				if err != nil || !strings.EqualFold(u.Host, r.Host) {
					writeJSON(w, 403, map[string]string{"error": "Cross-site requests are not allowed."})
					return
				}
			}
			if strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site") {
				writeJSON(w, 403, map[string]string{"error": "Cross-site requests are not allowed."})
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}
func (a *App) api(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	w.Header().Set("Cache-Control", "no-store")
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	before, _ := json.Marshal(a.state)
	result, status, err := a.route(w, r)
	if err == nil && r.Method != "GET" {
		if err = a.save(); err != nil {
			log.Printf("persist: %v", err)
			err = apiError{500, "Your changes could not be saved. Please try again."}
		}
	}
	if err != nil {
		json.Unmarshal(before, &a.state)
		w.Header().Del("Set-Cookie")
		code := 500
		message := "Something went wrong. Please try again."
		var e apiError
		if errors.As(err, &e) {
			code = e.status
			message = e.message
		}
		writeJSON(w, code, map[string]string{"error": message})
		return
	}
	writeJSON(w, status, result)
}
func validDate(s string) bool            { _, err := time.Parse("2006-01-02", s); return err == nil }
func textOK(s string, min, max int) bool { n := len(strings.TrimSpace(s)); return n >= min && n <= max }
func (a *App) route(w http.ResponseWriter, r *http.Request) (any, int, error) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/"), "/")
	parts := strings.Split(path, "/")
	u := a.user(r)
	if path == "health" && r.Method == "GET" {
		return map[string]string{"status": "ok"}, 200, nil
	}
	if path == "bootstrap" && r.Method == "GET" {
		return map[string]any{"user": publicUser(u), "listings": a.state.Listings, "needs": a.state.Needs, "demoMode": a.demo}, 200, nil
	}
	if path == "auth/register" && r.Method == "POST" {
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		if a.limited("register:" + ip) {
			return nil, 0, apiError{429, "Too many attempts. Try again in 15 minutes."}
		}
		var in struct {
			Name     string `json:"name"`
			Email    string `json:"email"`
			Password string `json:"password"`
			Role     string `json:"role"`
		}
		if err := decode(r, &in); err != nil {
			return nil, 0, err
		}
		in.Email = strings.ToLower(strings.TrimSpace(in.Email))
		addr, err := mail.ParseAddress(in.Email)
		if err != nil || addr.Address != in.Email || len(in.Email) > 254 || !textOK(in.Name, 2, 80) || len(in.Password) < 10 || len(in.Password) > 128 || (in.Role != "tenant" && in.Role != "landlord") {
			return nil, 0, bad("Use a valid name and email, choose your role, and enter a password of 10–128 characters.")
		}
		for _, v := range a.state.Users {
			if v.Email == in.Email {
				return nil, 0, apiError{409, "An account with this email already exists."}
			}
		}
		v := User{ID: id(), Name: strings.TrimSpace(in.Name), Email: in.Email, Role: in.Role, Salt: id(), Saved: []string{}}
		v.Hash = passwordHash(in.Password, v.Salt)
		a.state.Users = append(a.state.Users, v)
		a.login(w, v)
		return publicUser(&v), 201, nil
	}
	if path == "auth/login" && r.Method == "POST" {
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		if a.limited("login:" + ip) {
			return nil, 0, apiError{429, "Too many attempts. Try again in 15 minutes."}
		}
		var in struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := decode(r, &in); err != nil {
			return nil, 0, err
		}
		if len(in.Password) > 128 {
			return nil, 0, bad("Invalid credentials.")
		}
		for _, v := range a.state.Users {
			if v.Email == strings.ToLower(strings.TrimSpace(in.Email)) {
				if subtle.ConstantTimeCompare([]byte(v.Hash), []byte(passwordHash(in.Password, v.Salt))) == 1 {
					a.login(w, v)
					return publicUser(&v), 200, nil
				}
				return nil, 0, apiError{401, "The email or password is incorrect."}
			}
		}
		passwordHash(in.Password, "constant-time-dummy-salt")
		return nil, 0, apiError{401, "The email or password is incorrect."}
	}
	if path == "auth/demo" && r.Method == "POST" {
		if !a.demo {
			return nil, 0, apiError{404, "Demo accounts are disabled."}
		}
		var in struct {
			Role string `json:"role"`
		}
		if err := decode(r, &in); err != nil {
			return nil, 0, err
		}
		for _, v := range a.state.Users {
			if v.ID == "demo-"+in.Role {
				a.login(w, v)
				return publicUser(&v), 200, nil
			}
		}
		return nil, 0, bad("Choose tenant or landlord.")
	}
	if path == "auth/logout" && r.Method == "POST" {
		if c, err := r.Cookie("gk_session"); err == nil {
			delete(a.state.Sessions, sessionKey(c.Value))
		}
		http.SetCookie(w, &http.Cookie{Name: "gk_session", Value: "", Path: "/", HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
		return map[string]bool{"ok": true}, 200, nil
	}
	if path == "listings" && r.Method == "GET" {
		return a.state.Listings, 200, nil
	}
	if path == "needs" && r.Method == "GET" {
		return a.state.Needs, 200, nil
	}
	if u == nil {
		return nil, 0, apiError{401, "Please sign in to continue."}
	}
	if path == "me" && r.Method == "GET" {
		return publicUser(u), 200, nil
	}
	if len(parts) == 3 && parts[0] == "listings" && parts[2] == "save" && r.Method == "POST" {
		found := false
		for _, l := range a.state.Listings {
			if l.ID == parts[1] {
				found = true
			}
		}
		if !found {
			return nil, 0, apiError{404, "Home not found."}
		}
		if i := slices.Index(u.Saved, parts[1]); i >= 0 {
			u.Saved = slices.Delete(u.Saved, i, i+1)
		} else {
			u.Saved = append(u.Saved, parts[1])
		}
		return publicUser(u), 200, nil
	}
	if path == "listings" && r.Method == "POST" {
		if u.Role != "landlord" {
			return nil, 0, forbidden()
		}
		var l Listing
		if err := decode(r, &l); err != nil {
			return nil, 0, err
		}
		if !textOK(l.Title, 5, 100) || !textOK(l.City, 2, 60) || !textOK(l.Area, 2, 100) || !textOK(l.Address, 5, 300) || !textOK(l.Description, 20, 3000) || l.Rent < 500 || l.Rent > 1000000 || l.Deposit < 0 || l.Deposit > 10000000 || l.Beds < 1 || l.Beds > 20 || l.Baths < 1 || l.Baths > 20 || l.Size < 50 || l.Size > 20000 || l.MinMonths < 1 || l.MinMonths > 36 || !validDate(l.Available) || !slices.Contains([]string{"Shared room", "Private room", "Apartment"}, l.Type) {
			return nil, 0, bad("Check the property details, price, size, and availability date.")
		}
		if l.Image != "" {
			img, err := url.Parse(l.Image)
			if err != nil || img.Scheme != "https" || img.Host != "images.unsplash.com" || len(l.Image) > 1000 {
				return nil, 0, bad("Use an HTTPS images.unsplash.com photo URL or leave the photo empty.")
			}
		} else {
			l.Image = defaultImage
		}
		if len(l.Amenities) > 10 {
			return nil, 0, bad("Choose up to 10 amenities.")
		}
		for _, s := range l.Amenities {
			if !slices.Contains([]string{"Wi-Fi", "Kitchen", "Air conditioning", "Washing machine", "Parking", "Balcony", "Power backup"}, s) {
				return nil, 0, bad("Choose amenities from the available options.")
			}
		}
		l.ID = id()
		l.OwnerID = u.ID
		l.OwnerName = u.Name
		l.CreatedAt = now()
		a.state.Listings = append([]Listing{l}, a.state.Listings...)
		return l, 201, nil
	}
	if path == "needs" && r.Method == "POST" {
		if u.Role != "tenant" {
			return nil, 0, forbidden()
		}
		var n Need
		if err := decode(r, &n); err != nil {
			return nil, 0, err
		}
		if !textOK(n.Title, 5, 100) || !textOK(n.City, 2, 60) || !textOK(n.Description, 20, 2000) || n.Budget < 500 || n.Budget > 1000000 || n.Months < 1 || n.Months > 36 || !validDate(n.MoveIn) {
			return nil, 0, bad("Check your title, city, budget, dates, and description.")
		}
		n.ID = id()
		n.TenantID = u.ID
		n.TenantName = u.Name
		n.CreatedAt = now()
		a.state.Needs = append([]Need{n}, a.state.Needs...)
		return n, 201, nil
	}
	if path == "requests" && r.Method == "GET" {
		out := []Request{}
		for _, v := range a.state.Requests {
			if v.TenantID == u.ID || v.LandlordID == u.ID {
				out = append(out, v)
			}
		}
		return out, 200, nil
	}
	if path == "requests" && r.Method == "POST" {
		if u.Role != "tenant" {
			return nil, 0, forbidden()
		}
		var in struct {
			ListingID string `json:"listingId"`
			Message   string `json:"message"`
		}
		if err := decode(r, &in); err != nil {
			return nil, 0, err
		}
		if !textOK(in.Message, 10, 1500) {
			return nil, 0, bad("Introduce yourself in 10–1,500 characters.")
		}
		for _, v := range a.state.Requests {
			if v.TenantID == u.ID && v.ListingID == in.ListingID {
				return nil, 0, apiError{409, "You already sent a request for this home."}
			}
		}
		for _, l := range a.state.Listings {
			if l.ID == in.ListingID {
				if l.OwnerID == u.ID {
					return nil, 0, bad("You cannot request your own property.")
				}
				v := Request{ID: id(), ListingID: l.ID, TenantID: u.ID, LandlordID: l.OwnerID, TenantName: u.Name, LandlordName: l.OwnerName, Title: l.Title, Message: strings.TrimSpace(in.Message), Status: "pending", CreatedAt: now()}
				a.state.Requests = append([]Request{v}, a.state.Requests...)
				return v, 201, nil
			}
		}
		return nil, 0, apiError{404, "Home not found."}
	}
	if len(parts) >= 2 && parts[0] == "requests" {
		var req *Request
		for i := range a.state.Requests {
			if a.state.Requests[i].ID == parts[1] {
				req = &a.state.Requests[i]
			}
		}
		if req == nil {
			return nil, 0, apiError{404, "Request not found."}
		}
		if req.TenantID != u.ID && req.LandlordID != u.ID {
			return nil, 0, forbidden()
		}
		if len(parts) == 2 && r.Method == "PATCH" {
			if req.LandlordID != u.ID {
				return nil, 0, forbidden()
			}
			var in struct {
				Status string `json:"status"`
			}
			if err := decode(r, &in); err != nil {
				return nil, 0, err
			}
			if req.Status != "pending" {
				return nil, 0, apiError{409, "This request has already been reviewed."}
			}
			if in.Status != "accepted" && in.Status != "declined" {
				return nil, 0, bad("Accept or decline the request.")
			}
			req.Status = in.Status
			return req, 200, nil
		}
		if len(parts) == 3 && parts[2] == "messages" {
			if req.Status != "accepted" {
				return nil, 0, apiError{403, "Chat opens when the landlord accepts the request."}
			}
			if r.Method == "GET" {
				out := []Message{}
				for _, v := range a.state.Messages {
					if v.RequestID == req.ID {
						out = append(out, v)
					}
				}
				return out, 200, nil
			}
			if r.Method == "POST" {
				var in struct {
					Text string `json:"text"`
				}
				if err := decode(r, &in); err != nil {
					return nil, 0, err
				}
				if !textOK(in.Text, 1, 3000) {
					return nil, 0, bad("Enter a message of up to 3,000 characters.")
				}
				v := Message{ID: id(), RequestID: req.ID, SenderID: u.ID, SenderName: u.Name, Text: strings.TrimSpace(in.Text), CreatedAt: now()}
				a.state.Messages = append(a.state.Messages, v)
				return v, 201, nil
			}
		}
		if len(parts) == 3 && parts[2] == "agreement" {
			if req.Status != "accepted" {
				return nil, 0, apiError{403, "Agreements become available after acceptance."}
			}
			var ag *Agreement
			for i := range a.state.Agreements {
				if a.state.Agreements[i].RequestID == req.ID {
					ag = &a.state.Agreements[i]
				}
			}
			if r.Method == "GET" {
				return ag, 200, nil
			}
			if r.Method == "POST" {
				if ag != nil {
					return nil, 0, apiError{409, "An agreement draft already exists for this request."}
				}
				var in Agreement
				if err := decode(r, &in); err != nil {
					return nil, 0, err
				}
				if !validDate(in.Start) || !validDate(in.End) || in.End <= in.Start || in.Rent < 500 || in.Rent > 1000000 || in.Deposit < 0 || in.Deposit > 10000000 || !textOK(in.Terms, 10, 5000) {
					return nil, 0, bad("Check the dates, rent, deposit, and agreement terms.")
				}
				for _, l := range a.state.Listings {
					if l.ID == req.ListingID {
						in.Address = l.Address + ", " + l.Area + ", " + l.City
					}
				}
				in.ID = id()
				in.RequestID = req.ID
				in.Title = req.Title
				in.LandlordName = req.LandlordName
				in.TenantName = req.TenantName
				in.CreatedAt = now()
				in.Acknowledged = []string{}
				a.state.Agreements = append(a.state.Agreements, in)
				return in, 201, nil
			}
			if r.Method == "PATCH" {
				if ag == nil {
					return nil, 0, apiError{404, "Create a draft first."}
				}
				if !slices.Contains(ag.Acknowledged, u.ID) {
					ag.Acknowledged = append(ag.Acknowledged, u.ID)
				}
				return ag, 200, nil
			}
		}
	}
	return nil, 0, apiError{404, "This action is not available."}
}

const defaultImage = "https://images.unsplash.com/photo-1522708323590-d24dbb6b0267?auto=format&fit=crop&w=1000&q=85"

func (a *App) seed() {
	a.state = State{Users: []User{}, Listings: []Listing{}, Needs: []Need{}, Requests: []Request{}, Messages: []Message{}, Agreements: []Agreement{}, Sessions: map[string]Session{}}
	if !a.demo {
		return
	}
	for _, v := range []struct{ role, name string }{{"landlord", "Meera Sharma"}, {"tenant", "Aarav Patel"}} {
		u := User{ID: "demo-" + v.role, Name: v.name, Email: v.role + "@demo.gharkiraya.local", Role: v.role, Salt: id(), Saved: []string{}}
		u.Hash = passwordHash(id(), u.Salt)
		a.state.Users = append(a.state.Users, u)
	}
	date := time.Now().Format("2006-01-02")
	photos := []string{defaultImage, "https://images.unsplash.com/photo-1484154218962-a197022b5858?auto=format&fit=crop&w=1000&q=85", "https://images.unsplash.com/photo-1502672260266-1c1ef2d93688?auto=format&fit=crop&w=1000&q=85", "https://images.unsplash.com/photo-1493809842364-78817add7ffb?auto=format&fit=crop&w=1000&q=85", "https://images.unsplash.com/photo-1560448204-e02f11c3d0e2?auto=format&fit=crop&w=1000&q=85", "https://images.unsplash.com/photo-1536376072261-38c75010e6c9?auto=format&fit=crop&w=1000&q=85"}
	for i, v := range []struct {
		title, city, area, kind string
		rent, beds, size, min   int
	}{{"The sunlit studio", "Bengaluru", "Koramangala, 5th Block", "Private room", 12500, 1, 420, 1}, {"A little room, a lot of possibility", "Bengaluru", "Indiranagar, 100 Feet Road", "Shared room", 7500, 2, 680, 1}, {"Your own quiet corner", "Bengaluru", "HSR Layout, Sector 2", "Apartment", 18000, 2, 950, 3}, {"The balcony hideaway", "Pune", "Viman Nagar", "Private room", 9500, 1, 480, 1}, {"Room for your next big thing", "Delhi", "North Campus", "Shared room", 6500, 2, 600, 1}, {"A home near the action", "Hyderabad", "Gachibowli", "Apartment", 16000, 2, 900, 2}} {
		a.state.Listings = append(a.state.Listings, Listing{ID: strconv.Itoa(i + 1), OwnerID: "demo-landlord", OwnerName: "Meera Sharma", Title: v.title, City: v.city, Area: v.area, Address: fmt.Sprintf("%d, Greenview Residency", 12+i), Type: v.kind, Rent: v.rent, Deposit: v.rent * 2, Beds: v.beds, Baths: 1, Size: v.size, MinMonths: v.min, Available: date, Furnished: true, Amenities: []string{"Wi-Fi", "Kitchen", "Washing machine", "Balcony"}, Image: photos[i], Description: "A thoughtfully furnished space with plenty of natural light, a comfortable study corner, and room to make it your own. Close to cafés, public transport, and everyday essentials. Students and interns are welcome. Utilities are discussed directly with the owner.", CreatedAt: now()})
	}
	a.state.Needs = append(a.state.Needs, Need{ID: "need-demo", TenantID: "demo-tenant", TenantName: "Aarav Patel", Title: "A place to call home for my internship", City: "Bengaluru", Budget: 12000, Months: 3, MoveIn: time.Now().AddDate(0, 0, 15).Format("2006-01-02"), Description: "Moving to Bengaluru for a design internship. Looking for a furnished room around Koramangala or HSR Layout with Wi-Fi and a kitchen. Quiet, tidy, and always up for a good coffee.", CreatedAt: now()})
}
func main() {
	path := os.Getenv("DATA_FILE")
	if path == "" {
		path = "data/state.json"
	}
	a, err := newApp(path, os.Getenv("DEMO_MODE") == "true", os.Getenv("COOKIE_SECURE") == "true")
	if err != nil {
		log.Fatal(err)
	}
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	server := &http.Server{Addr: addr, Handler: a.handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	log.Printf("GharKiraya is ready at http://%s (demo mode: %t)", addr, a.demo)
	log.Fatal(server.ListenAndServe())
}
