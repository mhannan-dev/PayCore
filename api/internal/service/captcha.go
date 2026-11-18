package service

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"time"
)

const captchaTTL = 2 * time.Minute

type challenge struct {
	digest  [32]byte
	expires time.Time
}
type CaptchaStore struct {
	mu      sync.Mutex
	entries map[string]challenge
	now     func() time.Time
}

// CaptchaChallenge is a math question ("7 + 5 = ?") drawn as a distorted SVG.
// The answer is never sent to the client; only its salted digest is stored.
type CaptchaChallenge struct {
	ID        string `json:"captcha_id"`
	Image     string `json:"image"`
	ExpiresIn int    `json:"expires_in"`
}

func NewCaptchaStore() *CaptchaStore {
	return &CaptchaStore{entries: make(map[string]challenge), now: time.Now}
}

func randomHex() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

// randInt returns a uniform value in [lo, hi].
func randInt(lo, hi int) (int, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(hi-lo+1)))
	if err != nil {
		return 0, err
	}
	return lo + int(n.Int64()), nil
}

// mathQuestion picks an addition, subtraction (non-negative result) or small
// multiplication and returns its display text and answer.
func mathQuestion() (string, int, error) {
	op, err := randInt(0, 2)
	if err != nil {
		return "", 0, err
	}
	switch op {
	case 0:
		a, err1 := randInt(1, 20)
		b, err2 := randInt(1, 20)
		if err := errors.Join(err1, err2); err != nil {
			return "", 0, err
		}
		return fmt.Sprintf("%d + %d", a, b), a + b, nil
	case 1:
		a, err1 := randInt(5, 30)
		b, err2 := randInt(1, a)
		if err := errors.Join(err1, err2); err != nil {
			return "", 0, err
		}
		return fmt.Sprintf("%d - %d", a, b), a - b, nil
	default:
		a, err1 := randInt(2, 9)
		b, err2 := randInt(2, 9)
		if err := errors.Join(err1, err2); err != nil {
			return "", 0, err
		}
		return fmt.Sprintf("%d × %d", a, b), a * b, nil
	}
}

// renderMathSVG draws each character with its own jitter and rotation over
// noise lines and dots, so the text is not trivially copyable from the markup order.
func renderMathSVG(question string) (string, error) {
	const width, height = 200, 64
	palette := []string{"#1f4d36", "#245f43", "#3d6b2f", "#5b4a1f", "#2c3e6b", "#6b2c3e"}
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`, width, height, width, height)
	b.WriteString(`<rect width="100%" height="100%" fill="#f6f8f2"/>`)
	for i := 0; i < 5; i++ {
		x1, e1 := randInt(0, width)
		y1, e2 := randInt(0, height)
		x2, e3 := randInt(0, width)
		y2, e4 := randInt(0, height)
		c, e5 := randInt(0, len(palette)-1)
		if err := errors.Join(e1, e2, e3, e4, e5); err != nil {
			return "", err
		}
		fmt.Fprintf(&b, `<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="%s" stroke-opacity="0.35" stroke-width="1.5"/>`, x1, y1, x2, y2, palette[c])
	}
	for i := 0; i < 30; i++ {
		x, e1 := randInt(0, width)
		y, e2 := randInt(0, height)
		if err := errors.Join(e1, e2); err != nil {
			return "", err
		}
		fmt.Fprintf(&b, `<circle cx="%d" cy="%d" r="1" fill="#7b8e7b" fill-opacity="0.5"/>`, x, y)
	}
	chars := []rune(question + " = ?")
	step := float64(width-24) / float64(len(chars))
	for i, ch := range chars {
		if ch == ' ' {
			continue
		}
		dy, e1 := randInt(-6, 6)
		rot, e2 := randInt(-18, 18)
		c, e3 := randInt(0, len(palette)-1)
		size, e4 := randInt(24, 30)
		if err := errors.Join(e1, e2, e3, e4); err != nil {
			return "", err
		}
		x := 14 + step*float64(i) + step/2
		y := 42 + dy
		fmt.Fprintf(&b, `<text x="%.1f" y="%d" transform="rotate(%d %.1f %d)" font-family="Verdana,Arial,sans-serif" font-size="%d" font-weight="700" fill="%s" text-anchor="middle">%s</text>`,
			x, y, rot, x, y, size, palette[c], string(ch))
	}
	b.WriteString(`</svg>`)
	return b.String(), nil
}

func (s *CaptchaStore) Issue() (CaptchaChallenge, error) {
	id, err := randomHex()
	if err != nil {
		return CaptchaChallenge{}, err
	}
	question, answer, err := mathQuestion()
	if err != nil {
		return CaptchaChallenge{}, err
	}
	svg, err := renderMathSVG(question)
	if err != nil {
		return CaptchaChallenge{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for key, entry := range s.entries {
		if !now.Before(entry.expires) {
			delete(s.entries, key)
		}
	}
	if len(s.entries) >= 4096 {
		return CaptchaChallenge{}, errors.New("captcha capacity reached")
	}
	s.entries[id] = challenge{digest: answerDigest(id, strconv.Itoa(answer)), expires: now.Add(captchaTTL)}
	return CaptchaChallenge{
		ID:        id,
		Image:     "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg)),
		ExpiresIn: int(captchaTTL.Seconds()),
	}, nil
}

func answerDigest(id, answer string) [32]byte {
	return sha256.Sum256([]byte(id + ":" + answer))
}

// Verify consumes the challenge on the first attempt, including a wrong answer.
// Leading zeros and surrounding spaces in the answer are ignored ("07" == "7").
func (s *CaptchaStore) Verify(id, answer string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[id]
	delete(s.entries, id)
	if !ok || !s.now().Before(entry.expires) {
		return false
	}
	n, err := strconv.Atoi(strings.TrimSpace(answer))
	if err != nil || n < 0 {
		return false
	}
	actual := answerDigest(id, strconv.Itoa(n))
	return subtle.ConstantTimeCompare(actual[:], entry.digest[:]) == 1
}
