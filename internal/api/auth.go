package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"strings"

	"github.com/gofiber/fiber/v3"
)

// KeyAuth returns a middleware that requires one of the given API keys. With
// no keys configured it lets every request through (Piston compatible).
//
// The key is read from "Authorization: Bearer <key>", the "X-API-Key" header
// or - for WebSocket clients which cannot set headers - the "api_key" query
// parameter. The paths in open are not protected.
func KeyAuth(keys []string, open ...string) fiber.Handler {
	if len(keys) == 0 {
		return func(c fiber.Ctx) error { return c.Next() }
	}
	hashes := make([][32]byte, len(keys))
	for i, k := range keys {
		hashes[i] = sha256.Sum256([]byte(k))
	}
	skip := map[string]bool{}
	for _, p := range open {
		skip[p] = true
	}

	return func(c fiber.Ctx) error {
		if skip[c.Path()] {
			return c.Next()
		}
		got := extractKey(c)
		if got != "" {
			sum := sha256.Sum256([]byte(got))
			ok := 0
			for _, h := range hashes { // compare against all keys, in constant time
				ok |= subtle.ConstantTimeCompare(sum[:], h[:])
			}
			if ok == 1 {
				return c.Next()
			}
		}
		c.Set("WWW-Authenticate", `Bearer realm="cfm-sendbox"`)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "Invalid or missing API key"})
	}
}

func extractKey(c fiber.Ctx) string {
	if h := c.Get("Authorization"); len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	if h := c.Get("X-API-Key"); h != "" {
		return h
	}
	return c.Query("api_key")
}
