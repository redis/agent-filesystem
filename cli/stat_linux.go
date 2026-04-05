//go:build linux

package main

import "syscall"

func statAtime(st *syscall.Stat_t) (sec, nsec int64) {
	return st.Atim.Sec, st.Atim.Nsec
}

func statMtime(st *syscall.Stat_t) (sec, nsec int64) {
	return st.Mtim.Sec, st.Mtim.Nsec
}
