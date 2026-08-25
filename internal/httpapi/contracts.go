package httpapi

// Package-level transport contracts intentionally live beside handlers. The API
// uses service commands directly because their JSON tags form the versioned v1
// request schema; responses are always wrapped with requestId and data/error.
const APIVersion = "v1"
