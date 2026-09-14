//go:build windows

package workerbroker

// The Docker broker is unavailable on Windows; keeping the package buildable
// lets the coordination daemon continue to use remote approved brokers there.
const noFollowFlag = 0
