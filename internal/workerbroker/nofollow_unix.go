//go:build !windows

package workerbroker

import "syscall"

const noFollowFlag = syscall.O_NOFOLLOW
