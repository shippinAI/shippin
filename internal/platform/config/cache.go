package config

import (
	"errors"
	"net/url"
	"regexp"
)

func (c *Config) checkCache() error {
	if c.RedisURL == "" {
		return nil
	}
	u, err := url.Parse(c.RedisURL)
	if err != nil || (u.Scheme != "redis" && u.Scheme != "rediss") || u.Hostname() == "" {
		return errors.New("SHIPPIN_REDIS_URL must be a redis:// or rediss:// connection URL")
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9:_-]{1,80}$`).MatchString(c.RedisPrefix) {
		return errors.New("SHIPPIN_REDIS_PREFIX must contain 1-80 letters, digits, colons, underscores or hyphens")
	}
	return nil
}
