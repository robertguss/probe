package engine

import "time"

const (
	defaultTimeout = 30 * time.Second
	defaultMaxWait = 60 * time.Second
	defaultMaxBody = 1 << 20
	defaultRetries = 2
)
