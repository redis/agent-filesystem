//go:build darwin

package main

import "syscall"

func statAtime(st *syscall.Stat_t) (sec, nsec int64) {
	return st.Atimespec.Sec, int64(st.Atimespec.Nsec)
}

func statMtime(st *syscall.Stat_t) (sec, nsec int64) {
	return st.Mtimespec.Sec, int64(st.Mtimespec.Nsec)
}
